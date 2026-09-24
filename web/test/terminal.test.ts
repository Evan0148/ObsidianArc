import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createApp, h, nextTick, ref, type App } from 'vue';
import fs from 'node:fs';
import path from 'node:path';
import { createMemoryHistory, createRouter } from 'vue-router';
import TerminalPanel from '../src/views/TerminalPanel.vue';
import { providePanelHost } from '../src/composables/usePanelHost';
import { changeLanguage, t } from '../src/composables/useI18n';
import type { ConsoleExecHandlers, ConsoleSpec } from '../src/api/console';

// `TerminalPanel.vue` and `session.ts` both talk to the server through
// `api/console.ts` alone, so mocking that one module is enough to drive the
// whole page without a network stub — the shape `terminal-plumbing.test.ts`
// (the other half of this feature) already exercises the real client
// against.
vi.mock('../src/api/console', () => ({
  fetchConsoleSpec: vi.fn(),
  completeConsoleLine: vi.fn(),
  execConsoleLine: vi.fn(),
}));

import { completeConsoleLine, execConsoleLine, fetchConsoleSpec } from '../src/api/console';

const BANNER = 'Obsidian Arc console — vtest';

function makeSpec(overrides: Partial<ConsoleSpec> = {}): ConsoleSpec {
  return {
    version: 'vtest',
    you: { username: 'ada', role: 'super_admin', permissions: ['users'] },
    ssh: { enabled: true, addr: ':2222', fingerprint: 'SHA256:test' },
    banner: BANNER,
    commands: [],
    ...overrides,
  };
}

/** Answers every `run()` with one output chunk and a successful `done`. */
function mockExec(outputText: string): void {
  vi.mocked(execConsoleLine).mockImplementation(async (_request, handlers: ConsoleExecHandlers) => {
    handlers.onOut?.(outputText);
    handlers.onDone?.({ ok: true, code: '', exit: false, elapsed_ms: 1, json: false, lang: 'en' });
  });
}

let app: App | undefined;
let host: HTMLElement;

async function settle(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
  await nextTick();
}

async function mountTerminal(): Promise<void> {
  app = createApp({
    setup() {
      // AppShell provides the row a side panel becomes a column of; the
      // terminal is one, and its appearance settings are a second. A plain
      // element rather than a template ref, set once and never cleared, for
      // the reason AGENTS.md gives about refs during teardown.
      const row = document.createElement('div');
      host.appendChild(row);
      providePanelHost(ref(row));
      return () => h(TerminalPanel);
    },
  });
  // Closing the panel navigates back to the chat.
  app.use(createRouter({ history: createMemoryHistory(), routes: [{ path: '/:p(.*)*', component: { render: () => null } }] }));
  app.mount(host);
  await settle();
  await settle();
}

function input(node: HTMLInputElement | HTMLTextAreaElement, value: string): void {
  node.value = value;
  node.dispatchEvent(new Event('input', { bubbles: true }));
}

function key(node: Element, keyName: string): void {
  node.dispatchEvent(new KeyboardEvent('keydown', { key: keyName, bubbles: true, cancelable: true }));
}

beforeEach(async () => {
  await changeLanguage('en');
  vi.mocked(fetchConsoleSpec).mockResolvedValue(makeSpec());
  vi.mocked(completeConsoleLine).mockResolvedValue({ from: 0, items: [] });
  mockExec('42 users');
  host = document.createElement('div');
  document.body.appendChild(host);
  try {
    localStorage.clear();
  } catch {
    // A private window throws on clear(); nothing to do about it here either.
  }
});

afterEach(() => {
  app?.unmount();
  app = undefined;
  document.body.textContent = '';
  vi.mocked(fetchConsoleSpec).mockReset();
  vi.mocked(completeConsoleLine).mockReset();
  vi.mocked(execConsoleLine).mockReset();
  try {
    localStorage.clear();
  } catch {
    // See beforeEach.
  }
});

describe('the admin terminal', () => {
  it('prints the spec banner as the first block', async () => {
    await mountTerminal();
    const banner = host.querySelector('.oa-terminal-block-banner');
    expect(banner?.textContent).toContain(BANNER);
  });

  it('echoes a submitted line and shows its output', async () => {
    mockExec('42 users');
    await mountTerminal();

    const textarea = host.querySelector<HTMLTextAreaElement>('.oa-terminal-input')!;
    input(textarea, 'user list');
    key(textarea, 'Enter');
    await settle();
    await settle();

    const blocks = Array.from(host.querySelectorAll('.oa-terminal-block'));
    expect(blocks.some((block) => block.classList.contains('oa-terminal-block-echo') && block.textContent?.includes('user list'))).toBe(true);
    expect(blocks.some((block) => block.classList.contains('oa-terminal-block-output') && block.textContent?.includes('42 users'))).toBe(true);
    expect(vi.mocked(execConsoleLine)).toHaveBeenCalledOnce();
    // The draft clears once the line is sent, the way a shell's prompt does.
    expect(textarea.value).toBe('');
  });

  it('renders output incrementally across multiple stream chunks without truncation', async () => {
    vi.mocked(execConsoleLine).mockImplementation(async (_request, handlers: ConsoleExecHandlers) => {
      handlers.onOut?.('Accounts\n');
      await nextTick();
      handlers.onOut?.('Groups\n');
      await nextTick();
      handlers.onOut?.('Session\n');
      handlers.onDone?.({ ok: true, code: '', exit: false, elapsed_ms: 20, json: false, lang: 'en' });
    });
    await mountTerminal();

    const textarea = host.querySelector<HTMLTextAreaElement>('.oa-terminal-input')!;
    input(textarea, 'help');
    key(textarea, 'Enter');
    await settle();
    await settle();

    const outputBlock = host.querySelector('.oa-terminal-block-output');
    expect(outputBlock?.textContent).toContain('Accounts');
    expect(outputBlock?.textContent).toContain('Groups');
    expect(outputBlock?.textContent).toContain('Session');
  });

  it('keeps the tab strip inside the card at ten tabs, with the settings button always reachable', async () => {
    await mountTerminal();

    const addButton = host.querySelector<HTMLButtonElement>('.oa-terminal-tab-add')!;
    for (let i = 0; i < 9; i += 1) {
      addButton.click();
      await nextTick();
    }
    expect(host.querySelectorAll('.oa-terminal-tab')).toHaveLength(10);

    const strip = host.querySelector('.oa-terminal-tabstrip')!;
    const scroller = host.querySelector('.oa-terminal-tabs')!;
    const settingsButton = host.querySelector('.oa-terminal-settings-btn')!;
    const stillAddButton = host.querySelector('.oa-terminal-tab-add')!;

    // Pinned outside the scrolling row — a sibling, not a descendant — so it
    // stays reachable regardless of how many tabs overflow the scroller.
    expect(scroller.contains(settingsButton)).toBe(false);
    expect(strip.contains(settingsButton)).toBe(true);
    // "+" lives inside the same scroller as the tabs, per CONTRACT.md #5.2.
    expect(scroller.contains(stillAddButton)).toBe(true);

    // The layout rule that makes tabs shrink instead of pushing the strip
    // wider than its card, read from source: jsdom never lays anything out,
    // so a `scrollWidth` assertion here would only be measuring zeroes.
    // `styles.test.ts` already covers the equivalent built-CSS assertion for
    // screens that need one; this one does not depend on `make web` having
    // just run.
    const source = fs.readFileSync(path.resolve(__dirname, '../src/styles/_terminal.scss'), 'utf8');
    const tabRule = /\.oa-terminal-tab\s*\{[^}]*\}/.exec(source)?.[0] ?? '';
    expect(tabRule).toMatch(/flex:\s*0 1 150px/);
    expect(tabRule).toMatch(/min-width:\s*64px/);
    const scrollerRule = /\.oa-terminal-tabs\s*\{[^}]*\}/.exec(source)?.[0] ?? '';
    expect(scrollerRule).toMatch(/overflow-x:\s*auto/);
  });

  it('renames a tab on double-click, cancels a rename with Escape, and closes a tab with its ×', async () => {
    await mountTerminal();
    host.querySelector<HTMLButtonElement>('.oa-terminal-tab-add')!.click();
    await nextTick();
    expect(host.querySelectorAll('.oa-terminal-tab')).toHaveLength(2);

    const firstTab = host.querySelectorAll<HTMLElement>('.oa-terminal-tab')[0]!;
    firstTab.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }));
    await nextTick();
    const renameInput = firstTab.querySelector<HTMLInputElement>('.oa-terminal-tab-rename input')!;
    input(renameInput, 'Diagnostics');
    key(renameInput, 'Enter');
    await nextTick();
    expect(firstTab.querySelector('.oa-terminal-tab-label')?.textContent).toBe('Diagnostics');

    // A second rename, abandoned with Escape, leaves the committed title alone.
    firstTab.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }));
    await nextTick();
    const secondAttempt = firstTab.querySelector<HTMLInputElement>('.oa-terminal-tab-rename input')!;
    input(secondAttempt, 'Abandoned');
    key(secondAttempt, 'Escape');
    await nextTick();
    expect(firstTab.querySelector('.oa-terminal-tab-label')?.textContent).toBe('Diagnostics');

    const closeButtons = host.querySelectorAll<HTMLButtonElement>('.oa-terminal-tab-close');
    closeButtons[1]!.click();
    await nextTick();
    expect(host.querySelectorAll('.oa-terminal-tab')).toHaveLength(1);
    expect(host.querySelector('.oa-terminal-tab-label')?.textContent).toBe('Diagnostics');
  });

  it('opens a fresh tab when the only one closes, rather than leaving an empty screen', async () => {
    await mountTerminal();
    expect(host.querySelectorAll('.oa-terminal-tab')).toHaveLength(1);

    host.querySelector<HTMLButtonElement>('.oa-terminal-tab-close')!.click();
    await nextTick();

    const tabs = host.querySelectorAll('.oa-terminal-tab');
    expect(tabs).toHaveLength(1);
    // A new tab, not the one that just closed still lingering: its default
    // label numbers past it, since tab serials are never reused.
    expect(tabs[0]?.querySelector('.oa-terminal-tab-label')?.textContent).toBe(t('terminalTabDefault', { n: 2 }));
    expect(host.querySelector('.oa-terminal-input')).not.toBeNull();
  });

  it('opens appearance as a column beside the terminal, previewing live and persisting on commit', async () => {
    await mountTerminal();

    host.querySelector<HTMLButtonElement>('.oa-terminal-settings-btn')!.click();
    await nextTick();
    await new Promise(requestAnimationFrame);

    // A column of the chat row, not a sheet over the terminal: it is a
    // sibling of the terminal's own card rather than a descendant of it,
    // which is the whole difference between the two.
    const panel = Array.from(host.querySelectorAll<HTMLElement>('.oa-panel'))
      .find((node) => node.querySelector('.oa-terminal-settings')) ?? null;
    expect(panel).not.toBeNull();
    const terminal = host.querySelector<HTMLElement>('.oa-terminal')!;
    expect(terminal.contains(panel)).toBe(false);
    expect(panel!.querySelector('.oa-terminal-settings')).not.toBeNull();

    expect(terminal.style.getPropertyValue('--oa-term-font-size')).toBe('13px');

    const fontSizeRange = host.querySelector<HTMLInputElement>('.oa-terminal-settings input[type="range"]')!;
    fontSizeRange.value = '18';
    fontSizeRange.dispatchEvent(new Event('input', { bubbles: true }));
    await nextTick();
    // Live preview: applied to the page before anything is written to disk.
    expect(terminal.style.getPropertyValue('--oa-term-font-size')).toBe('18px');
    expect(localStorage.getItem('obsidian-arc-terminal-prefs')).toBeNull();

    fontSizeRange.dispatchEvent(new Event('change', { bubbles: true }));
    await nextTick();
    const stored = JSON.parse(localStorage.getItem('obsidian-arc-terminal-prefs') ?? '{}') as { fontSize?: number };
    expect(stored.fontSize).toBe(18);

    // Using a field does not dismiss the column. It never could, now that it
    // is a column — which is the point of it being one.
    expect(host.querySelector('.oa-panel')).not.toBeNull();
  });

  it('mounts and unmounts cleanly, aborting anything still running', async () => {
    // Still "running" when the page goes away — the shape teardown has to
    // survive without throwing.
    vi.mocked(execConsoleLine).mockImplementation(() => new Promise(() => {}));

    await mountTerminal();
    host.querySelector<HTMLButtonElement>('.oa-terminal-settings-btn')!.click();
    await nextTick();
    host.querySelector<HTMLButtonElement>('.oa-terminal-tab-add')!.click();
    await nextTick();

    const textarea = host.querySelector<HTMLTextAreaElement>('.oa-terminal-input')!;
    input(textarea, 'dash');
    key(textarea, 'Enter');
    await nextTick();

    expect(() => app?.unmount()).not.toThrow();
    app = undefined;
    const signal = vi.mocked(execConsoleLine).mock.calls[0]?.[2];
    expect(signal?.aborted).toBe(true);
    expect(host.textContent).toBe('');
  });
});
