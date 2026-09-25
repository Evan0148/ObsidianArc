// A notification's feedback_new link (/admin/feedback/<id>) has to land on
// that one thread, not on the unfiltered list — AdminPage keeps AdminFeedback
// mounted across that navigation (same slug, a different trailing segment),
// so this exercises both the first visit and a second link clicked while the
// page is already open, the one onMounted alone cannot catch.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createApp, h, nextTick, reactive, shallowRef, type App } from 'vue';
import { adminApi } from '../src/admin/api';
import type { Feedback, FeedbackThread } from '../src/api/feedback';
import { changeLanguage } from '../src/composables/useI18n';
import { providePanelHost } from '../src/composables/usePanelHost';
import AdminFeedback from '../src/views/admin/AdminFeedback.vue';
import { provideAdminView } from '../src/views/admin/adminView';

let app: App | undefined;
let host: HTMLElement;
let panels: HTMLElement;
let actions: HTMLElement;

beforeEach(async () => {
  await changeLanguage('en');
  host = document.createElement('div');
  panels = document.createElement('div');
  actions = document.createElement('div');
  document.body.append(host, panels, actions);
});

afterEach(() => {
  app?.unmount();
  app = undefined;
  document.body.textContent = '';
  vi.restoreAllMocks();
});

async function settle(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
  await nextTick();
}

function record(over: Partial<Feedback> = {}): Feedback {
  return {
    id: 'f1', user_id: 'u1', kind: 'bug', priority: 'high', status: 'open',
    title: 'Streaming stops', body: 'It stops half way.',
    created_at: Date.now(), updated_at: Date.now(), username: 'reader', nickname: 'Reader',
    replies: 0, author_unread: false, operator_unread: true,
    ...over,
  };
}

function thread(over: Partial<FeedbackThread> = {}): FeedbackThread {
  return { feedback: record(), replies: [], ...over };
}

function emptyList() {
  return vi.spyOn(adminApi, 'feedback').mockResolvedValue({
    feedback: [], total: 0, offset: 0,
    summary: { total: 0, open: 0, bugs: 0, ideas: 0, high_open: 0, awaiting: 0 },
  });
}

/**
 * Mounts the page behind a `params` getter that reads a reactive object,
 * the same way AdminPage's own `params` reads its route-derived `segments`
 * computed — so re-assigning `state.params` here is what a second
 * notification link changes underneath the mounted page.
 */
function mountWithParams(initial: string[]): { params: string[] } {
  const state = reactive({ params: initial });
  app = createApp({
    setup() {
      providePanelHost(shallowRef(panels));
      provideAdminView({
        actionsHost: actions,
        setTitle: () => {},
        reload: () => {},
        get params() {
          return state.params;
        },
      });
      return () => h(AdminFeedback);
    },
  });
  app.mount(host);
  return state;
}

describe('opening a feedback thread from its id in the URL', () => {
  it('opens the thread on mount, with no list row to show first', async () => {
    emptyList();
    const fetchThread = vi.spyOn(adminApi, 'feedbackThread').mockResolvedValue(thread());
    mountWithParams(['f1']);
    await settle();

    expect(fetchThread).toHaveBeenCalledWith('f1');
    expect(panels.textContent).toContain('Streaming stops');
  });

  it('shows the deep-linked report\'s own row as selected once the list arrives', async () => {
    vi.spyOn(adminApi, 'feedback').mockResolvedValue({
      feedback: [record({ operator_unread: true })], total: 1, offset: 0,
      summary: { total: 1, open: 1, bugs: 1, ideas: 0, high_open: 0, awaiting: 1 },
    });
    vi.spyOn(adminApi, 'feedbackThread').mockResolvedValue(thread());
    mountWithParams(['f1']);
    await settle();

    const card = host.querySelector('.oa-feedback-card')!;
    expect(card.classList.contains('selected')).toBe(true);
  });

  it('opens a second thread when the id in the params changes without a remount', async () => {
    emptyList();
    const fetchThread = vi.spyOn(adminApi, 'feedbackThread')
      .mockResolvedValueOnce(thread({ feedback: record({ id: 'f1', title: 'First report' }) }))
      .mockResolvedValueOnce(thread({ feedback: record({ id: 'f2', title: 'Second report' }) }));
    const state = mountWithParams(['f1']);
    await settle();
    expect(panels.textContent).toContain('First report');

    // AdminPage does not remount the page for this — same slug, a new
    // trailing segment — so only the watch on params catches it.
    state.params = ['f2'];
    await settle();

    expect(fetchThread).toHaveBeenCalledTimes(2);
    expect(fetchThread).toHaveBeenLastCalledWith('f2');
    expect(panels.textContent).toContain('Second report');
    expect(panels.textContent).not.toContain('First report');
  });

  it('leaves the list on its own when no id is in the params', async () => {
    const list = emptyList();
    const fetchThread = vi.spyOn(adminApi, 'feedbackThread');
    mountWithParams([]);
    await settle();

    expect(list).toHaveBeenCalled();
    expect(fetchThread).not.toHaveBeenCalled();
    expect(panels.textContent ?? '').toBe('');
  });
});
