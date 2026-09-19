import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createApp, h, nextTick, shallowRef, type App, type Component } from 'vue';
import { adminApi } from '../src/admin/api';
import * as feedbackApi from '../src/api/feedback';
import type { Feedback } from '../src/api/feedback';
import { providePanelHost } from '../src/composables/usePanelHost';
import { site, siteInfo } from '../src/stores/session';
import { changeLanguage, t } from '../src/composables/useI18n';
import FeedbackPanel from '../src/views/FeedbackPanel.vue';
import AdminFeedback from '../src/views/admin/AdminFeedback.vue';
import { provideAdminView } from '../src/views/admin/adminView';

vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }));

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
  site.value = null;
});

async function settle(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
  await nextTick();
}

async function mount(component: Component): Promise<void> {
  app = createApp({
    setup() {
      providePanelHost(shallowRef(panels));
      provideAdminView({ actionsHost: actions, setTitle: () => {}, reload: () => {}, params: [] });
      return () => h(component);
    },
  });
  app.mount(host);
  await settle();
}

function button(root: ParentNode, label: string): HTMLButtonElement {
  const found = [...root.querySelectorAll<HTMLButtonElement>('button')]
    .find((node) => node.textContent?.trim() === label);
  if (!found) throw new Error(`Button not found: ${label}`);
  return found;
}

function type(node: HTMLInputElement | HTMLTextAreaElement, value: string): void {
  node.value = value;
  node.dispatchEvent(new Event('input', { bubbles: true }));
}

function record(over: Partial<Feedback> = {}): Feedback {
  return {
    id: 'f1', user_id: 'u1', kind: 'bug', priority: 'high', status: 'open',
    title: 'Streaming stops', body: 'It stops half way.\n\nEvery time.',
    created_at: Date.now(), updated_at: Date.now(), username: 'reader', nickname: 'Reader',
    ...over,
  };
}

describe('the feedback panel somebody writes in', () => {
  it('sends the type, priority, title and body that were chosen', async () => {
    vi.spyOn(feedbackApi, 'listFeedback').mockResolvedValue({ feedback: [], remaining: 10, max_per_day: 10 });
    const send = vi.spyOn(feedbackApi, 'sendFeedback').mockResolvedValue(record());
    await mount(FeedbackPanel);

    button(panels, t('feedbackKindIdea')).click();
    button(panels, t('feedbackPriorityHigh')).click();
    type(panels.querySelector<HTMLInputElement>('input[type="text"]')!, '  A darker terminal  ');
    type(panels.querySelector<HTMLTextAreaElement>('textarea')!, 'It would help at night.');
    await nextTick();

    button(panels, t('feedbackSend')).click();
    await settle();

    expect(send).toHaveBeenCalledWith({
      kind: 'idea', priority: 'high',
      title: 'A darker terminal', body: 'It would help at night.',
      turnstile: '',
    });
    // The confirmation stays up, and the form is empty and ready for another.
    expect(panels.textContent).toContain(t('feedbackSent'));
    expect(panels.querySelector<HTMLTextAreaElement>('textarea')!.value).toBe('');
  });

  it('refuses an empty title or body without asking the server', async () => {
    vi.spyOn(feedbackApi, 'listFeedback').mockResolvedValue({ feedback: [], remaining: 10, max_per_day: 10 });
    const send = vi.spyOn(feedbackApi, 'sendFeedback');
    await mount(FeedbackPanel);

    button(panels, t('feedbackSend')).click();
    await settle();
    expect(send).not.toHaveBeenCalled();
    expect(panels.textContent).toContain(t('feedbackNeedTitle'));

    type(panels.querySelector<HTMLInputElement>('input[type="text"]')!, 'A title');
    button(panels, t('feedbackSend')).click();
    await settle();
    expect(send).not.toHaveBeenCalled();
    expect(panels.textContent).toContain(t('feedbackNeedBody'));
  });

  it('shows what this account has already sent, with the status it was given', async () => {
    vi.spyOn(feedbackApi, 'listFeedback').mockResolvedValue({
      feedback: [record({ status: 'resolved', title: 'Already reported' })],
      remaining: 9, max_per_day: 10,
    });
    await mount(FeedbackPanel);

    expect(panels.textContent).toContain('Already reported');
    expect(panels.textContent).toContain(t('feedbackStatusResolved'));
    expect(panels.textContent).toContain(t('feedbackRemaining', { count: 9 }));
  });

  // Off unless the operator switched it on: an account that is already
  // signed in and already capped does not meet a challenge by default.
  it('draws the challenge only in the scene the operator switched it on for', async () => {
    vi.spyOn(feedbackApi, 'listFeedback').mockResolvedValue({ feedback: [], remaining: 10, max_per_day: 10 });
    await mount(FeedbackPanel);
    expect(panels.querySelector('.oa-challenge')).toBeNull();
    app?.unmount();
    app = undefined;
    panels.textContent = '';

    site.value = { ...siteInfo.value, turnstile_on_feedback: true, turnstile_site_key: '1x000' };
    await mount(FeedbackPanel);
    expect(panels.querySelector('.oa-challenge')).not.toBeNull();
  });

  // The server refuses past the cap either way; the point of asking first is
  // that nobody types five hundred words into a box that cannot be sent.
  it('withdraws the send button once the day is used up', async () => {
    vi.spyOn(feedbackApi, 'listFeedback').mockResolvedValue({ feedback: [], remaining: 0, max_per_day: 10 });
    await mount(FeedbackPanel);

    expect(panels.textContent).toContain(t('feedbackNoneLeft'));
    expect(() => button(panels, t('feedbackSend'))).toThrow();
  });
});

describe('the operator’s feedback page', () => {
  const summary = { total: 2, open: 1, bugs: 1, ideas: 1, high_open: 1 };

  function list(rows: Feedback[]) {
    return vi.spyOn(adminApi, 'feedback').mockResolvedValue({
      feedback: rows, total: rows.length, offset: 0, summary,
    });
  }

  it('lists reports with their author and counts what is still waiting', async () => {
    list([record(), record({ id: 'f2', kind: 'idea', priority: 'low', status: 'resolved', title: 'A darker theme' })]);
    await mount(AdminFeedback);

    const cards = host.querySelectorAll('.oa-feedback-card');
    expect(cards).toHaveLength(2);
    expect(cards[0]!.textContent).toContain('Streaming stops');
    expect(cards[0]!.textContent).toContain(t('feedbackFrom', { name: 'Reader' }));
    expect(cards[1]!.classList.contains('resolved')).toBe(true);
    expect(host.querySelector('.oa-stat-grid')!.textContent).toContain(t('feedbackStatHigh'));
  });

  it('opens one report whole and resolves it without editing what was written', async () => {
    list([record()]);
    const patch = vi.spyOn(adminApi, 'setFeedbackStatus').mockResolvedValue(record({ status: 'resolved' }));
    await mount(AdminFeedback);

    host.querySelector<HTMLButtonElement>('.oa-feedback-card')!.click();
    await settle();
    // Newlines and all: the blank line in the middle is half of what was said.
    expect(panels.querySelector('.oa-feedback-detail-body')!.textContent)
      .toBe('It stops half way.\n\nEvery time.');

    button(panels, t('feedbackResolve')).click();
    await settle();
    expect(patch).toHaveBeenCalledWith('f1', 'resolved');
  });

  it('narrows the list by status without leaving the page it was on', async () => {
    const reads = list([record()]);
    await mount(AdminFeedback);
    reads.mockClear();

    const statusSelect = [...host.querySelectorAll<HTMLElement>('.oa-field')]
      .find((field) => field.querySelector('.oa-field-label')?.textContent === t('colStatus'))!;
    statusSelect.querySelector<HTMLButtonElement>('.oa-select')!.click();
    await settle();
    [...document.querySelectorAll<HTMLElement>('[role="option"]')]
      .find((item) => item.textContent?.trim() === t('feedbackStatusOpen'))!.click();
    await settle();

    expect(reads).toHaveBeenCalledWith(expect.stringContaining('status=open'));
  });
});
