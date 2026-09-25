import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { createApp, h, nextTick, ref, type App } from 'vue';
import { createMemoryHistory, createRouter } from 'vue-router';
import UsageBoard from '../src/views/admin/usage/UsageBoard.vue';
import ChatToolCall from '../src/chat/ChatToolCall.vue';
import ChatSurface from '../src/chat/ChatSurface.vue';
import AdminDashboard from '../src/views/admin/AdminDashboard.vue';
import OaMenu from '../src/components/OaMenu.vue';
import { adminApi } from '../src/admin/api';
import { provideAdminView } from '../src/views/admin/adminView';
import { dashboardFixture } from './fixtures/dashboard';
import { pendingMode, setMode } from '../src/stores/workspace';
import { providePanelHost } from '../src/composables/usePanelHost';

let app: App | null = null;
let host: HTMLElement;

beforeEach(() => {
  document.body.textContent = '';
  host = document.createElement('div');
  document.body.appendChild(host);
});

afterEach(() => {
  app?.unmount();
  app = null;
  document.body.textContent = '';
});

describe('Non-linear animation & expand/collapse transitions', () => {
  it('UsageBoard expands and collapses extra rows with animated accordion and chevron', async () => {
    const rows = Array.from({ length: 10 }, (_, i) => ({
      key: `model-${i}`,
      label: `Model ${i}`,
      requests: 100 - i,
      total_tokens: 1000 * (10 - i),
      errors: 0,
    }));

    app = createApp({
      render() {
        return h(UsageBoard as any, {
          rows,
          kind: 'model',
          metric: 'requests',
          limit: 6,
          emptyText: 'No models',
        });
      },
    });
    app.mount(host);
    await nextTick();

    // Limit is 6, total 10. The toggle button should be present.
    const moreBtn = host.querySelector<HTMLButtonElement>('.oa-board-more');
    expect(moreBtn).not.toBeNull();
    expect(moreBtn?.classList.contains('open')).toBe(false);

    const chevron = moreBtn?.querySelector('.oa-board-more-chevron');
    expect(chevron).not.toBeNull();
    expect(chevron?.classList.contains('open')).toBe(false);

    // Accordion container should initially not have .open
    const collapseContainer = host.querySelector('.oa-board-collapse');
    expect(collapseContainer).not.toBeNull();
    expect(collapseContainer?.classList.contains('open')).toBe(false);

    // Top rows are in the main list
    const topRowItems = host.querySelectorAll('.oa-board > li:not(.oa-board-collapse-item)');
    expect(topRowItems.length).toBe(6);

    // Extra rows are inside the collapse container
    const extraRowItems = host.querySelectorAll('.oa-board-sublist > li');
    expect(extraRowItems.length).toBe(4);

    // Click "显示全部"
    moreBtn?.click();
    await nextTick();

    expect(moreBtn?.classList.contains('open')).toBe(true);
    expect(chevron?.classList.contains('open')).toBe(true);
    expect(collapseContainer?.classList.contains('open')).toBe(true);

    // Click "收起"
    moreBtn?.click();
    await nextTick();

    expect(moreBtn?.classList.contains('open')).toBe(false);
    expect(chevron?.classList.contains('open')).toBe(false);
    expect(collapseContainer?.classList.contains('open')).toBe(false);
  });

  it('ChatToolCall toggles smoothly with aria-expanded and open state', async () => {
    const call = {
      id: 'tool-1',
      name: 'read_file',
      arguments: '{"path": "file.txt"}',
      output: 'file contents',
      done: true,
      failed: false,
    };

    app = createApp({
      render() {
        return h(ChatToolCall, { call });
      },
    });
    app.mount(host);
    await nextTick();

    const container = host.querySelector('.ai-tool-call');
    const head = host.querySelector<HTMLButtonElement>('.ai-tool-call-head');
    const content = host.querySelector('.ai-tool-call-content');

    expect(container).not.toBeNull();
    expect(head).not.toBeNull();
    expect(content).not.toBeNull();
    expect(container?.classList.contains('open')).toBe(false);
    expect(head?.getAttribute('aria-expanded')).toBe('false');

    // Click to expand
    head?.click();
    await nextTick();

    expect(container?.classList.contains('open')).toBe(true);
    expect(head?.getAttribute('aria-expanded')).toBe('true');

    // Click to collapse
    head?.click();
    await nextTick();

    expect(container?.classList.contains('open')).toBe(false);
    expect(head?.getAttribute('aria-expanded')).toBe('false');
  });

  it('ChatSurface mode switch displays sliding pill corresponding to pendingMode', async () => {
    const { models, selectedID } = await import('../src/chat/useModels');
    models.value = [{ id: 'test-model', display_name: 'Test Model' } as any];
    selectedID.value = 'test-model';

    setMode('chat');
    expect(pendingMode.value).toBe('chat');

    app = createApp({
      setup() {
        providePanelHost(ref(host));
        return () => h(ChatSurface);
      },
    });
    app.mount(host);
    await nextTick();

    const pill = host.querySelector('.ai-mode-pill');
    expect(pill).not.toBeNull();
    expect(pill?.classList.contains('mode-work')).toBe(false);

    const suggestionsAccordion = host.querySelector('.ai-chat-suggestions-accordion');
    expect(suggestionsAccordion?.classList.contains('open')).toBe(true);

    // Switch to work mode
    setMode('work');
    await nextTick();

    expect(pill?.classList.contains('mode-work')).toBe(true);
    expect(suggestionsAccordion?.classList.contains('open')).toBe(false);

    // Switch back to chat mode
    setMode('chat');
    await nextTick();

    expect(pill?.classList.contains('mode-work')).toBe(false);
    expect(suggestionsAccordion?.classList.contains('open')).toBe(true);
  });

  it('AdminDashboard ranking details expands and collapses with non-linear animation and rotating chevron', async () => {
    vi.spyOn(adminApi, 'dashboard').mockResolvedValue(dashboardFixture());
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { render: () => null } }] });
    const actions = document.createElement('div');
    app = createApp({
      setup() {
        provideAdminView({ actionsHost: actions, setTitle() {}, reload() {}, params: [] });
        return () => h(AdminDashboard);
      },
    });
    app.use(router);
    app.mount(host);
    await new Promise((r) => setTimeout(r, 0));
    await nextTick();

    const ranking = host.querySelector('#secBusiestModels')!;
    expect(ranking).not.toBeNull();

    const details = ranking.querySelector('details.oa-dashboard-ranking-details');
    expect(details).not.toBeNull();

    const summary = details?.querySelector('summary.oa-dashboard-details-summary');
    expect(summary).not.toBeNull();

    const chevron = summary?.querySelector('.oa-dashboard-details-chevron');
    expect(chevron).not.toBeNull();
    expect(chevron?.classList.contains('open')).toBe(false);

    const collapse = details?.querySelector('.oa-dashboard-details-collapse');
    expect(collapse).not.toBeNull();
    expect(collapse?.classList.contains('open')).toBe(false);

    // Click summary to expand
    (summary as HTMLElement).click();
    await nextTick();

    expect(chevron?.classList.contains('open')).toBe(true);
    expect(collapse?.classList.contains('open')).toBe(true);

    // Click summary again to collapse
    (summary as HTMLElement).click();
    await nextTick();

    expect(chevron?.classList.contains('open')).toBe(false);
    expect(collapse?.classList.contains('open')).toBe(false);
  });

  it('OaMenu mounts and triggers open animation with non-linear transform', async () => {
    app = createApp({
      render() {
        return h(OaMenu as any, null, {
          trigger: ({ toggle }: any) => h('button', { class: 'trigger-btn', onClick: toggle }, 'Menu'),
          default: () => h('div', { class: 'menu-content' }, 'Content'),
        });
      },
    });
    app.mount(host);
    await nextTick();

    // Menu panel is not mounted before trigger is clicked
    expect(host.querySelector('.oa-menu')).toBeNull();

    // Click trigger to open
    const trigger = host.querySelector<HTMLButtonElement>('.trigger-btn')!;
    trigger.click();
    await nextTick();

    // Menu panel should be mounted
    const panel = host.querySelector<HTMLElement>('.oa-menu');
    expect(panel).not.toBeNull();

    // requestAnimationFrame transitions shown to true
    await new Promise((r) => requestAnimationFrame(r));
    await nextTick();
    expect(panel?.classList.contains('open')).toBe(true);
  });
});
