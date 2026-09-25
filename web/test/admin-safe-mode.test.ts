import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createApp, h, nextTick, type App } from 'vue';
import { createMemoryHistory, createRouter } from 'vue-router';
import { adminApi } from '../src/admin/api';
import { provideAdminView } from '../src/views/admin/adminView';
import AdminDashboard from '../src/views/admin/AdminDashboard.vue';
import UsageBoard from '../src/views/admin/usage/UsageBoard.vue';
import { dashboardFixture } from './fixtures/dashboard';
import {
  activeMaskCount,
  isMasked,
  mask,
  maskBilling,
  maskCredential,
  maskLog,
  maskProvider,
  maskUser,
  SAFE_CATEGORIES,
  safeModeCategories,
  safeModeEnabled,
  setAllCategories,
  STORAGE_KEY,
  toggleCategory,
  toggleSafeMode,
  MASK_PLACEHOLDER,
} from '../src/admin/safeMode';
import AdminSafeMode from '../src/views/admin/AdminSafeMode.vue';

describe('Admin Safe Mode store & utilities', () => {
  beforeEach(() => {
    localStorage.clear();
    safeModeEnabled.value = false;
    for (const cat of SAFE_CATEGORIES) {
      safeModeCategories.value[cat.key] = true;
    }
  });

  afterEach(() => {
    localStorage.clear();
    safeModeEnabled.value = false;
  });

  it('masks nothing when safe mode is disabled', () => {
    safeModeEnabled.value = false;
    expect(isMasked('users')).toBe(false);
    expect(isMasked('providers')).toBe(false);
    expect(isMasked('credentials')).toBe(false);
    expect(isMasked('billing')).toBe(false);
    expect(isMasked('logs')).toBe(false);

    expect(maskUser('alice')).toBe('alice');
    expect(maskProvider('OpenAI')).toBe('OpenAI');
    expect(maskCredential('sk-test1234')).toBe('sk-test1234');
    expect(maskBilling('100,000')).toBe('100,000');
    expect(maskLog('192.168.1.1')).toBe('192.168.1.1');
  });

  it('masks values into ****** when safe mode is enabled', () => {
    toggleSafeMode(true);
    expect(safeModeEnabled.value).toBe(true);

    expect(isMasked('users')).toBe(true);
    expect(isMasked('providers')).toBe(true);
    expect(isMasked('credentials')).toBe(true);
    expect(isMasked('billing')).toBe(true);
    expect(isMasked('logs')).toBe(true);

    expect(maskUser('alice')).toBe(MASK_PLACEHOLDER);
    expect(maskProvider('Anthropic')).toBe(MASK_PLACEHOLDER);
    expect(maskCredential('sk-proj-xxxx')).toBe(MASK_PLACEHOLDER);
    expect(maskBilling('5,000')).toBe(MASK_PLACEHOLDER);
    expect(maskLog('10.0.0.1')).toBe(MASK_PLACEHOLDER);
  });

  it('allows selective category masking', () => {
    toggleSafeMode(true);
    toggleCategory('providers', false);
    toggleCategory('billing', false);

    expect(isMasked('users')).toBe(true);
    expect(isMasked('providers')).toBe(false);
    expect(isMasked('billing')).toBe(false);
    expect(isMasked('credentials')).toBe(true);

    expect(maskUser('alice')).toBe(MASK_PLACEHOLDER);
    expect(maskProvider('OpenAI')).toBe('OpenAI');
    expect(maskBilling('1,000')).toBe('1,000');
    expect(maskCredential('sk-xxxx')).toBe(MASK_PLACEHOLDER);
  });

  it('handles empty, null, and undefined values cleanly', () => {
    toggleSafeMode(true);
    expect(mask(null, 'users')).toBe('');
    expect(mask(undefined, 'users')).toBe('');
    expect(mask('', 'users')).toBe('');
    expect(maskUser(null)).toBe('');
    expect(maskUser(undefined)).toBe('');
  });

  it('supports setAllCategories and computes activeMaskCount', () => {
    toggleSafeMode(true);
    expect(activeMaskCount.value).toBe(5);

    setAllCategories(false);
    expect(activeMaskCount.value).toBe(0);
    expect(isMasked('users')).toBe(false);

    setAllCategories(true);
    expect(activeMaskCount.value).toBe(5);
    expect(isMasked('users')).toBe(true);
  });

  it('persists preferences to localStorage', async () => {
    toggleSafeMode(true);
    toggleCategory('users', false);
    await nextTick();

    const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!);
    expect(stored.enabled).toBe(true);
    expect(stored.categories.users).toBe(false);
    expect(stored.categories.providers).toBe(true);
  });
});

describe('AdminSafeMode component', () => {
  let app: App | undefined;
  let host: HTMLElement;

  beforeEach(() => {
    localStorage.clear();
    safeModeEnabled.value = false;
    setAllCategories(true);
    host = document.createElement('div');
    document.body.appendChild(host);
  });

  afterEach(() => {
    app?.unmount();
    app = undefined;
    host.remove();
    localStorage.clear();
    safeModeEnabled.value = false;
  });

  it('renders eye toggle button and toggles menu', async () => {
    app = createApp(AdminSafeMode);
    app.mount(host);

    const triggerBtn = host.querySelector<HTMLButtonElement>('.oa-safe-mode-btn')!;
    expect(triggerBtn).not.toBeNull();
    expect(triggerBtn.classList.contains('is-active')).toBe(false);

    // Open dropdown menu
    triggerBtn.click();
    await nextTick();

    // Menu content is present
    const menu = host.querySelector('.oa-menu-safe-mode')!;
    expect(menu).not.toBeNull();
    expect(menu.textContent).toContain('******');

    // Toggle master switch
    const masterSwitch = menu.querySelector<HTMLInputElement>('.oa-safe-switch input')!;
    expect(masterSwitch.checked).toBe(false);
    masterSwitch.click();
    await nextTick();

    expect(safeModeEnabled.value).toBe(true);
    expect(triggerBtn.classList.contains('is-active')).toBe(true);

    // Category rows can be toggled
    const categoryRows = menu.querySelectorAll<HTMLButtonElement>('.oa-safe-mode-row');
    expect(categoryRows.length).toBe(SAFE_CATEGORIES.length);

    // Click the first category row (users)
    categoryRows[0]!.click();
    await nextTick();
    expect(safeModeCategories.value.users).toBe(false);

    // Click select all
    const selectAllBtn = menu.querySelector<HTMLButtonElement>('.oa-safe-mode-action-btn')!;
    selectAllBtn.click();
    await nextTick();
    expect(safeModeCategories.value.users).toBe(true);
  });

  it('toggles safe mode when clicking header title area', async () => {
    app = createApp(AdminSafeMode);
    app.mount(host);

    const triggerBtn = host.querySelector<HTMLButtonElement>('.oa-safe-mode-btn')!;
    triggerBtn.click();
    await nextTick();

    const head = host.querySelector<HTMLElement>('.oa-safe-mode-head')!;
    expect(safeModeEnabled.value).toBe(false);

    head.click();
    await nextTick();
    expect(safeModeEnabled.value).toBe(true);

    head.click();
    await nextTick();
    expect(safeModeEnabled.value).toBe(false);
  });

  it('enables safe mode when clicking a category while safe mode is disabled', async () => {
    app = createApp(AdminSafeMode);
    app.mount(host);

    const triggerBtn = host.querySelector<HTMLButtonElement>('.oa-safe-mode-btn')!;
    triggerBtn.click();
    await nextTick();

    expect(safeModeEnabled.value).toBe(false);

    const categoryRows = host.querySelectorAll<HTMLButtonElement>('.oa-safe-mode-row');
    categoryRows[0]!.click();
    await nextTick();

    expect(safeModeEnabled.value).toBe(true);
    expect(safeModeCategories.value.users).toBe(true);
  });
});

describe('Admin Safe Mode live component integration', () => {
  let app: App | undefined;
  let host: HTMLElement;
  let actions: HTMLElement;

  beforeEach(() => {
    localStorage.clear();
    safeModeEnabled.value = false;
    setAllCategories(true);
    host = document.createElement('div');
    actions = document.createElement('div');
    document.body.append(host, actions);
  });

  afterEach(() => {
    app?.unmount();
    app = undefined;
    host.remove();
    actions.remove();
    localStorage.clear();
    safeModeEnabled.value = false;
    vi.restoreAllMocks();
  });

  it('dynamically masks and unmasks sensitive data in AdminDashboard when safe mode toggles', async () => {
    vi.spyOn(adminApi, 'dashboard').mockResolvedValue(dashboardFixture());
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { render: () => null } }] });
    app = createApp({
      setup() {
        provideAdminView({ actionsHost: actions, setTitle() {}, reload() {}, params: [] });
        return () => h(AdminDashboard);
      },
    });
    app.use(router);
    app.mount(host);
    await new Promise((resolve) => setTimeout(resolve, 20));
    await nextTick();

    // In ranking table, switch to users
    const rankingSection = host.querySelector('#secBusiestModels')!;
    const usersBtn = [...rankingSection.querySelectorAll<HTMLButtonElement>('button')].find((btn) => btn.textContent?.includes('Users') || btn.textContent?.includes('用户'));
    expect(usersBtn).toBeDefined();
    usersBtn?.click();
    await nextTick();

    // When safe mode is off, user name 'onyx' should be visible
    expect(rankingSection.textContent).toContain('onyx');

    // KPI cards have unmasked tokens and credits
    const metricsSection = host.querySelector('.oa-dashboard-metrics')!;
    expect(metricsSection.textContent).toContain('168.6M');
    expect(metricsSection.textContent).toContain('2M');

    // Recent requests section
    const recentSection = host.querySelector('#secRecentRequests')!;
    expect(recentSection.textContent).toContain('onyx');
    expect(recentSection.textContent).toContain('Anthropic');

    // Turn ON Safe Mode
    toggleSafeMode(true);
    await nextTick();

    // User name 'onyx' should be masked to '******'
    expect(rankingSection.querySelector('.oa-dashboard-rank-list')?.textContent).not.toContain('onyx');
    expect(rankingSection.querySelector('.oa-dashboard-rank-list')?.textContent).toContain('******');

    // KPI cards have masked tokens and credits
    expect(metricsSection.textContent).not.toContain('168.6M');
    expect(metricsSection.textContent).not.toContain('2M');

    // Provider name Anthropic should be masked to '******' in recent requests
    expect(recentSection.textContent).not.toContain('Anthropic');
    expect(recentSection.textContent).toContain('******');

    // Turn OFF Safe Mode
    toggleSafeMode(false);
    await nextTick();

    // 'onyx' and 'Anthropic' should be back
    expect(rankingSection.querySelector('.oa-dashboard-rank-list')?.textContent).toContain('onyx');
    expect(recentSection.textContent).toContain('Anthropic');
    expect(metricsSection.textContent).toContain('168.6M');
    expect(metricsSection.textContent).toContain('2M');
  });
});

describe('UsageBoard safe mode masking', () => {
  let app: App | undefined;
  let host: HTMLElement;

  beforeEach(() => {
    localStorage.clear();
    safeModeEnabled.value = false;
    setAllCategories(true);
    host = document.createElement('div');
    document.body.appendChild(host);
  });

  afterEach(() => {
    app?.unmount();
    app = undefined;
    host.remove();
    localStorage.clear();
    safeModeEnabled.value = false;
  });

  it('masks user names and @detail notes in UsageBoard', async () => {
    const dummyRows = [
      { key: 'user-1', label: 'Alice Wonder', detail: 'alicew', requests: 10, total_tokens: 5000, credits: 2.5 },
    ];
    app = createApp({
      render() {
        return h(UsageBoard as any, { rows: dummyRows, kind: 'user', metric: 'tokens', emptyText: 'none' });
      },
    });
    app.mount(host);
    await nextTick();

    expect(host.textContent).toContain('Alice Wonder');
    expect(host.textContent).toContain('@alicew');

    toggleSafeMode(true);
    await nextTick();

    expect(host.textContent).not.toContain('Alice Wonder');
    expect(host.textContent).not.toContain('@alicew');
    expect(host.textContent).toContain('******');
    expect(host.textContent).toContain('@******');
  });
});

