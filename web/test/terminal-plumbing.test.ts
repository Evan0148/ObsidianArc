// The non-visual half of the admin terminal: CONTRACT.md #5.1/#5.7.
//
// No component is mounted here — that is `terminal.test.ts`'s job, once the
// Vue half exists. This file is the ANSI tokeniser, the preferences store
// and the per-tab session model, each exercised directly.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  createAnsiTokenizer,
  initialAnsiState,
  tokenizeAnsiChunk,
} from '../src/terminal/ansi';
import {
  DEFAULT_TERMINAL_FONT_FAMILY,
  TERMINAL_CURSOR_STYLES,
  TERMINAL_PREFS_DEFAULTS,
  loadTerminalPrefs,
  saveTerminalPrefs,
  terminalCSSVariables,
  type TerminalPrefs,
} from '../src/terminal/prefs';
import {
  createTerminalSession,
  terminalHistorySnapshot,
  type TerminalSession,
} from '../src/terminal/session';

const PREFS_KEY = 'obsidian-arc-terminal-prefs';

// --- ansi.ts -----------------------------------------------------------------

describe('the ANSI tokeniser', () => {
  it('maps SGR reset/bold/dim/underline/inverse to classes', () => {
    const { tokens } = tokenizeAnsiChunk(
      '\x1b[1mbold\x1b[22m\x1b[2mdim\x1b[0m\x1b[4munderline\x1b[24m\x1b[7minverse\x1b[27mplain',
      initialAnsiState(),
    );
    expect(tokens).toEqual([
      { kind: 'text', text: 'bold', cls: ['ansi-bold'] },
      { kind: 'text', text: 'dim', cls: ['ansi-dim'] },
      { kind: 'text', text: 'underline', cls: ['ansi-underline'] },
      { kind: 'text', text: 'inverse', cls: ['ansi-inverse'] },
      { kind: 'text', text: 'plain', cls: [] },
    ]);
  });

  it('maps 30-37, 90-97 and 39 (default foreground)', () => {
    const { tokens } = tokenizeAnsiChunk('\x1b[31mred\x1b[91mbright red\x1b[39mdefault', initialAnsiState());
    expect(tokens).toEqual([
      { kind: 'text', text: 'red', cls: ['ansi-fg-31'] },
      { kind: 'text', text: 'bright red', cls: ['ansi-fg-91'] },
      { kind: 'text', text: 'default', cls: [] },
    ]);
  });

  it('combines bold with a colour in one class list', () => {
    const { tokens } = tokenizeAnsiChunk('\x1b[1;32mgreen bold', initialAnsiState());
    expect(tokens).toEqual([{ kind: 'text', text: 'green bold', cls: ['ansi-bold', 'ansi-fg-32'] }]);
  });

  it('recognises the screen-clear sequences as a distinct token kind', () => {
    const { tokens } = tokenizeAnsiChunk('before\x1b[H\x1b[2Jafter\x1b[3Jtail', initialAnsiState());
    expect(tokens).toEqual([
      { kind: 'text', text: 'before', cls: [] },
      { kind: 'clear', text: '', cls: [] },
      { kind: 'clear', text: '', cls: [] },
      { kind: 'text', text: 'after', cls: [] },
      { kind: 'clear', text: '', cls: [] },
      { kind: 'text', text: 'tail', cls: [] },
    ]);
  });

  it('swallows an unknown sequence rather than printing it', () => {
    // Cursor position report request — nothing render.go ever emits, but a
    // real terminal stream could still carry one.
    const { tokens } = tokenizeAnsiChunk('before\x1b[6nafter', initialAnsiState());
    expect(tokens).toEqual([{ kind: 'text', text: 'beforeafter', cls: [] }]);
  });

  it('carries an escape sequence split across two chunks', () => {
    // "\x1b[31m" (set red) cut right after the "3" — the exact seam an SSE
    // chunk boundary can land on, since it has no notion of an escape
    // sequence's length.
    const first = tokenizeAnsiChunk('plain \x1b[3', initialAnsiState());
    expect(first.tokens).toEqual([{ kind: 'text', text: 'plain ', cls: [] }]);
    expect(first.state.pending).toBe('\x1b[3');

    const second = tokenizeAnsiChunk('1mred', first.state);
    expect(second.tokens).toEqual([{ kind: 'text', text: 'red', cls: ['ansi-fg-31'] }]);
    expect(second.state.pending).toBe('');
  });

  it('carries a lone ESC at the very end of a chunk', () => {
    const first = tokenizeAnsiChunk('tail\x1b', initialAnsiState());
    expect(first.tokens).toEqual([{ kind: 'text', text: 'tail', cls: [] }]);
    expect(first.state.pending).toBe('\x1b');

    const second = tokenizeAnsiChunk('[32mgreen', first.state);
    expect(second.tokens).toEqual([{ kind: 'text', text: 'green', cls: ['ansi-fg-32'] }]);
  });

  it('a split clear sequence still clears once whole', () => {
    const first = tokenizeAnsiChunk('\x1b[2', initialAnsiState());
    expect(first.tokens).toEqual([]);
    const second = tokenizeAnsiChunk('Jgone', first.state);
    expect(second.tokens).toEqual([
      { kind: 'clear', text: '', cls: [] },
      { kind: 'text', text: 'gone', cls: [] },
    ]);
  });

  it('createAnsiTokenizer holds state across an arbitrary number of pushes', () => {
    const tokenizer = createAnsiTokenizer();
    expect(tokenizer.push('a\x1b[1')).toEqual([{ kind: 'text', text: 'a', cls: [] }]);
    expect(tokenizer.push('mb')).toEqual([{ kind: 'text', text: 'b', cls: ['ansi-bold'] }]);
    // Bold is still in effect — state persisted across the two pushes.
    expect(tokenizer.push('c')).toEqual([{ kind: 'text', text: 'c', cls: ['ansi-bold'] }]);
    tokenizer.reset();
    expect(tokenizer.push('d')).toEqual([{ kind: 'text', text: 'd', cls: [] }]);
  });
});

// --- prefs.ts ------------------------------------------------------------------

describe('terminal prefs', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
  });

  it('falls back to defaults when the key is absent', () => {
    expect(loadTerminalPrefs()).toEqual(TERMINAL_PREFS_DEFAULTS);
  });

  it('falls back to defaults when the value is unparseable', () => {
    localStorage.setItem(PREFS_KEY, '{not json');
    expect(loadTerminalPrefs()).toEqual(TERMINAL_PREFS_DEFAULTS);
  });

  it('round-trips a valid set of prefs', () => {
    const prefs: TerminalPrefs = {
      fontFamily: '"JetBrains Mono", monospace',
      fontSize: 16,
      lineHeight: 1.8,
      letterSpacing: 1,
      cursorStyle: 'underline',
      cursorBlink: false,
      scrollback: 10000,
      timestamps: true,
      ligatures: true,
    };
    saveTerminalPrefs(prefs);
    expect(loadTerminalPrefs()).toEqual(prefs);
  });

  it('clamps a hand-edited value that is out of range', () => {
    localStorage.setItem(
      PREFS_KEY,
      JSON.stringify({
        fontFamily: DEFAULT_TERMINAL_FONT_FAMILY,
        fontSize: 400,
        lineHeight: 99,
        letterSpacing: -5,
        cursorStyle: 'block',
        cursorBlink: true,
        scrollback: 2000,
        timestamps: false,
        ligatures: false,
      }),
    );
    const prefs = loadTerminalPrefs();
    expect(prefs.fontSize).toBe(20); // clamped to the max, not 400px
    expect(prefs.lineHeight).toBe(2.0);
    expect(prefs.letterSpacing).toBe(0);
  });

  it('falls back to a valid enum member when the stored one is not one of them', () => {
    localStorage.setItem(PREFS_KEY, JSON.stringify({ cursorStyle: 'triangle', scrollback: 12345 }));
    const prefs = loadTerminalPrefs();
    expect(TERMINAL_CURSOR_STYLES).toContain(prefs.cursorStyle);
    expect(prefs.cursorStyle).toBe(TERMINAL_PREFS_DEFAULTS.cursorStyle);
    expect(prefs.scrollback).toBe(TERMINAL_PREFS_DEFAULTS.scrollback);
  });

  it('rejects a font-family value that has clearly stopped being a font stack', () => {
    localStorage.setItem(PREFS_KEY, JSON.stringify({ fontFamily: 'x'.repeat(5000) }));
    expect(loadTerminalPrefs().fontFamily).toBe(DEFAULT_TERMINAL_FONT_FAMILY);
  });

  it('reads and writes through a localStorage that throws, without throwing itself', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError');
    });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError');
    });

    expect(() => loadTerminalPrefs()).not.toThrow();
    expect(loadTerminalPrefs()).toEqual(TERMINAL_PREFS_DEFAULTS);
    expect(() => saveTerminalPrefs(TERMINAL_PREFS_DEFAULTS)).not.toThrow();
  });

  it('exports the CSS custom properties the page root applies', () => {
    const vars = terminalCSSVariables(TERMINAL_PREFS_DEFAULTS);
    expect(vars['--oa-term-font-size']).toBe('13px');
    expect(vars['--oa-term-line-height']).toBe('1.5');
    expect(vars['--oa-term-cursor-style']).toBe('bar');
  });
});

// --- session.ts ----------------------------------------------------------------

function sseResponse(events: Array<{ event: string; data: unknown }>): Response {
  const body = events.map(({ event, data }) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`).join('');
  const bytes = new TextEncoder().encode(body);
  return new Response(
    new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(bytes);
        controller.close();
      },
    }),
    { status: 200, headers: { 'Content-Type': 'text/event-stream' } },
  );
}

/** A stream that never closes on its own — only `init.signal` aborting it ends the read, which is exactly what `cancel()` is being tested against. */
function pendingSSEResponse(init: RequestInit): Response {
  let streamController: ReadableStreamDefaultController<Uint8Array> | undefined;
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      streamController = controller;
    },
  });
  init.signal?.addEventListener(
    'abort',
    () => streamController?.error(new DOMException('The user aborted a request.', 'AbortError')),
    { once: true },
  );
  return new Response(stream, { status: 200, headers: { 'Content-Type': 'text/event-stream' } });
}

function stubFetch(impl: (url: string, init: RequestInit) => Response | Promise<Response>): void {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init: RequestInit = {}) => {
      const url = typeof input === 'string' ? input : input.toString();
      return Promise.resolve(impl(url, init));
    }),
  );
}

function makeSession(id: string, cap = 2000): TerminalSession {
  return createTerminalSession({ id, scrollbackCap: cap, getColumns: () => 100 });
}

describe('terminal session', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('runs a line, threading SSE output through the ANSI tokeniser into the scrollback', async () => {
    stubFetch((url) => {
      expect(url).toContain('/api/console/exec');
      return sseResponse([
        { event: 'out', data: { text: 'hello ' } },
        { event: 'out', data: { text: '\x1b[31mred\x1b[0m' } },
        { event: 'done', data: { ok: true, code: '', exit: false, elapsed_ms: 5 } },
      ]);
    });

    const session = makeSession('run-basic');
    const outcome = await session.run('user list');

    expect(outcome).toEqual({ ok: true, code: '', exit: false });
    const kinds = session.scrollback.value.map((b) => b.kind);
    expect(kinds).toEqual(['echo', 'output']);
    expect(session.scrollback.value[1]!.tokens).toEqual([
      { kind: 'text', text: 'hello ', cls: [] },
      { kind: 'text', text: 'red', cls: ['ansi-fg-31'] },
    ]);
  });

  it('clears the scrollback when the output carries a clear token, for watch', async () => {
    stubFetch(() =>
      sseResponse([
        { event: 'out', data: { text: 'frame one' } },
        { event: 'out', data: { text: '\x1b[H\x1b[2Jframe two' } },
        { event: 'done', data: { ok: true, code: '', exit: false, elapsed_ms: 5 } },
      ]),
    );

    const session = makeSession('run-watch');
    await session.run('watch dash');

    // The echo block from the submitted line is wiped too — a real terminal
    // clear does not spare its own prompt line either.
    const kinds = session.scrollback.value.map((b) => b.kind);
    expect(kinds).toEqual(['output']);
    expect(session.scrollback.value[0]!.tokens).toEqual([{ kind: 'text', text: 'frame two', cls: [] }]);
  });

  it('cancel() aborts the stream and resolves with a cancelled outcome, not a rejection', async () => {
    stubFetch((_url, init) => pendingSSEResponse(init));

    const session = makeSession('run-cancel');
    const running = session.run('watch dash');
    await Promise.resolve();
    await Promise.resolve();
    expect(session.running.value).toBe(true);

    session.cancel();
    await expect(running).resolves.toEqual({ ok: false, code: 'cancelled', exit: false });
    expect(session.running.value).toBe(false);
  });

  it('dispose() aborts an in-flight command the same way cancel() does', async () => {
    stubFetch((_url, init) => pendingSSEResponse(init));

    const session = makeSession('run-dispose');
    const running = session.run('watch dash');
    await Promise.resolve();
    await Promise.resolve();

    session.dispose();
    await expect(running).resolves.toEqual({ ok: false, code: 'cancelled', exit: false });
  });

  it('caps the scrollback at the configured block count', async () => {
    stubFetch(() => sseResponse([{ event: 'done', data: { ok: true, code: '', exit: false, elapsed_ms: 0 } }]));

    const session = makeSession('run-scrollback-cap', 5);
    // Sequential on purpose: one session runs one command at a time, so
    // awaiting inside the loop is what the real prompt does too.
    for (let i = 0; i < 10; i += 1) {
      await session.run(`echo block-${i}`);
    }

    expect(session.scrollback.value).toHaveLength(5);
    const texts = session.scrollback.value.map((b) => b.tokens.map((t) => t.text).join(''));
    expect(texts).toEqual(['echo block-5', 'echo block-6', 'echo block-7', 'echo block-8', 'echo block-9']);
  });

  it('lowers the cap immediately when the preference changes mid-session', async () => {
    stubFetch(() => sseResponse([{ event: 'done', data: { ok: true, code: '', exit: false, elapsed_ms: 0 } }]));

    const session = makeSession('run-scrollback-cap-live', 100);
    for (let i = 0; i < 8; i += 1) {
      await session.run(`echo line-${i}`);
    }
    expect(session.scrollback.value).toHaveLength(8);

    session.setScrollbackCap(3);
    expect(session.scrollback.value).toHaveLength(3);
    expect(session.scrollback.value[2]!.tokens[0]!.text).toBe('echo line-7');
  });

  it('collapses consecutive duplicate commands in shared history', async () => {
    stubFetch(() => sseResponse([{ event: 'done', data: { ok: true, code: '', exit: false, elapsed_ms: 0 } }]));

    const session = makeSession('history-dedupe');
    await session.run('terminal-plumbing-dup-check');
    await session.run('terminal-plumbing-dup-check');
    await session.run('terminal-plumbing-dup-check');

    const snapshot = terminalHistorySnapshot();
    expect(snapshot.filter((line) => line === 'terminal-plumbing-dup-check')).toHaveLength(1);
    expect(snapshot[snapshot.length - 1]).toBe('terminal-plumbing-dup-check');
  });

  it('caps shared history at 500 entries, evicting the oldest first', async () => {
    stubFetch(() => sseResponse([{ event: 'done', data: { ok: true, code: '', exit: false, elapsed_ms: 0 } }]));

    const session = makeSession('history-cap');
    const total = 505;
    for (let i = 0; i < total; i += 1) {
      await session.run(`cap-test-${String(i).padStart(4, '0')}`);
    }

    const snapshot = terminalHistorySnapshot();
    expect(snapshot.length).toBe(500);
    // The first 5 of this test's own pushes were evicted; nothing before
    // this test's run survives once 500 of its own distinct lines exist.
    expect(snapshot[0]).toBe('cap-test-0005');
    expect(snapshot[snapshot.length - 1]).toBe(`cap-test-${String(total - 1).padStart(4, '0')}`);
  });

  it('recall() walks history and restores the in-progress draft on the way back down', async () => {
    stubFetch(() => sseResponse([{ event: 'done', data: { ok: true, code: '', exit: false, elapsed_ms: 0 } }]));

    const session = makeSession('recall');
    await session.run('recall-cmd-one');
    await session.run('recall-cmd-two');

    session.draft.value = 'unsent draft';
    session.recall('up');
    expect(session.draft.value).toBe('recall-cmd-two');
    session.recall('up');
    expect(session.draft.value).toBe('recall-cmd-one');

    session.recall('down');
    expect(session.draft.value).toBe('recall-cmd-two');
    session.recall('down');
    expect(session.draft.value).toBe('unsent draft');
  });

  it('complete() resolves to an empty result rather than throwing on a network failure', async () => {
    stubFetch(() => {
      throw new Error('network down');
    });

    const session = makeSession('complete-failure');
    await expect(session.complete('user ed', 7)).resolves.toEqual({ from: 7, items: [] });
  });
});
