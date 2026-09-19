import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createApp, h, nextTick, type App } from 'vue';
import { createMemoryHistory, createRouter } from 'vue-router';
import { adminApi } from '../src/admin/api';
import AdminDashboard from '../src/views/admin/AdminDashboard.vue';
import AdminDashboardTrend from '../src/views/admin/AdminDashboardTrend.vue';
import { provideAdminView } from '../src/views/admin/adminView';
import { changeLanguage, t } from '../src/composables/useI18n';
import { dashboardFixture, emptyTotals } from './fixtures/dashboard';

let app: App | undefined;
let host: HTMLElement;
let actions: HTMLElement;
async function settle(): Promise<void> { await new Promise((resolve) => setTimeout(resolve, 0)); await nextTick(); }
beforeEach(async () => {
  await changeLanguage('en');
  host = document.createElement('div');
  actions = document.createElement('div');
  document.body.append(host, actions);
});
afterEach(() => { app?.unmount(); app = undefined; document.body.textContent = ''; vi.restoreAllMocks(); });
function button(root: ParentNode, label: string): HTMLButtonElement {
  const found = [...root.querySelectorAll<HTMLButtonElement>('button')].find((node) => node.textContent?.trim() === label || node.getAttribute('aria-label') === label);
  if (!found) throw new Error(`Missing button: ${label}`);
  return found;
}
async function mountDashboard(): Promise<void> {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { render: () => null } }] });
  app = createApp({ setup() {
    provideAdminView({ actionsHost: actions, setTitle() {}, reload() {}, params: [] });
    return () => h(AdminDashboard);
  } });
  app.use(router);
  app.mount(host);
  await settle();
}

describe('dashboard overview', () => {
  it('switches the ranking without losing names, exact figures or detailed request counts', async () => {
    vi.spyOn(adminApi, 'dashboard').mockResolvedValue(dashboardFixture());
    await mountDashboard();
    const ranking = host.querySelector('#secBusiestModels')!;
    expect(ranking.textContent).toContain('Claude Sonnet 4.5');
    expect(ranking.querySelectorAll('.oa-dashboard-rank-list li')).toHaveLength(5);
    expect(ranking.querySelector('details')?.textContent).toContain('840,000');
    button(ranking, t('statUsers')).click(); await nextTick();
    expect(ranking.querySelector('.oa-dashboard-rank-list')?.textContent).toContain('onyx');
    expect(ranking.textContent).not.toContain('Claude Sonnet 4.5');
    await changeLanguage('zh');
    expect(host.textContent).toContain('概览');
    expect(button(ranking, t('statUsers')).getAttribute('aria-pressed')).toBe('true');
  });

  it('keeps the last snapshot on refresh failure and allows a retry', async () => {
    const load = vi.spyOn(adminApi, 'dashboard').mockResolvedValueOnce(dashboardFixture()).mockRejectedValueOnce(new Error('Temporarily unavailable')).mockResolvedValueOnce({ ...dashboardFixture(), counts: { ...dashboardFixture().counts, users: 200 } });
    await mountDashboard();
    button(host, t('refresh')).click(); await settle();
    expect(host.textContent).toContain('Temporarily unavailable');
    expect(host.querySelector('.oa-dashboard-resource-copy')?.textContent).toContain('176');
    button(host, t('refresh')).click(); await settle();
    expect(host.textContent).not.toContain('Temporarily unavailable');
    expect(host.querySelector('.oa-dashboard-resource-copy')?.textContent).toContain('200');
    expect(load).toHaveBeenCalledTimes(3);
  });

  it('shows empty states and never invents a success rate for an unused instance', async () => {
    vi.spyOn(adminApi, 'dashboard').mockResolvedValue({ ...dashboardFixture(), last_24h: { ...emptyTotals }, last_7d: { ...emptyTotals }, series: [], top_models: [], top_users: [], recent: [] });
    await mountDashboard();
    expect(host.textContent).toContain(t('noRequestsWeek'));
    expect(host.textContent).toContain(t('noRequestsYet'));
    expect(host.textContent).not.toContain('100%');
    expect(host.textContent).not.toContain('NaN');
    expect(host.querySelector('[role="slider"]')).toBeNull();
  });
});

describe('dashboard trend', () => {
  it('explores real bucket values with the keyboard and switches to token totals', async () => {
    const data = dashboardFixture();
    app = createApp(AdminDashboardTrend, { series: data.series, totals: data.last_7d, bucketMs: data.bucket_ms });
    app.mount(host); await nextTick();
    const slider = host.querySelector<HTMLElement>('[role="slider"]')!;
    slider.focus();
    slider.dispatchEvent(new KeyboardEvent('keydown', { key: 'Home', bubbles: true })); await nextTick();
    expect(slider.getAttribute('aria-valuenow')).toBe('1');
    expect(host.querySelector('.oa-dashboard-trend-summary strong')?.textContent).toBe('120');
    slider.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true })); await nextTick();
    expect(slider.getAttribute('aria-valuenow')).toBe('1');
    slider.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true })); await nextTick();
    expect(host.querySelector('.oa-dashboard-trend-summary strong')?.textContent).toBe('180');
    slider.blur();
    button(host, t('statTokens')).click(); await nextTick();
    expect(host.textContent).toContain(t('dashboardWeekTotal'));
    expect(button(host, t('statTokens')).getAttribute('aria-pressed')).toBe('true');
    expect(host.querySelector('.oa-dashboard-trend-line')?.getAttribute('d')).not.toMatch(/NaN|Infinity/);
  });

  it('renders one bucket without dividing by zero and distinguishes zero tokens from zero requests', async () => {
    const point = { ...emptyTotals, at: Date.now(), requests: 1 };
    app = createApp(AdminDashboardTrend, { series: [point], totals: point, bucketMs: 21600000 });
    app.mount(host); await nextTick();
    expect(host.querySelector('.oa-dashboard-trend-line')?.getAttribute('d')).toBe('M 360.00 130.50');
    button(host, t('statTokens')).click(); await nextTick();
    expect(host.textContent).toContain(t('dashboardNoTokens'));
    expect(host.querySelector('[role="slider"]')).toBeNull();
  });
});
