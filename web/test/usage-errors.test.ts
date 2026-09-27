import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createApp, h, nextTick, ref, type App } from 'vue';
import { api } from '@/api/client';
import { providePanelHost } from '@/composables/usePanelHost';
import { formatErrorCode, errorCodeDescription } from '@/lib/errors';
import UsagePanel from '@/views/UsagePanel.vue';
import StatusBadge from '@/views/admin/StatusBadge.vue';

vi.mock('@/api/client', () => ({
  ApiError: class ApiError extends Error {},
  api: { get: vi.fn(), post: vi.fn() },
}));
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }));

describe('error reason formatting', () => {
  it('formats known error codes into readable labels', () => {
    expect(formatErrorCode('')).toBe('');
    expect(formatErrorCode(undefined)).toBe('');
    expect(formatErrorCode('provider_rate_limited')).toBe('Rate limited');
    expect(formatErrorCode('provider_unreachable')).toBe('Unreachable');
    expect(formatErrorCode('provider_auth')).toBe('Auth error');
    expect(formatErrorCode('provider_error')).toBe('Provider error');
    expect(formatErrorCode('quota_exceeded')).toBe('Quota exceeded');
    expect(formatErrorCode('timeout')).toBe('Timeout');
  });

  it('provides detailed explanation sentences for known error codes', () => {
    expect(errorCodeDescription('provider_rate_limited')).toContain('rate limiting requests');
    expect(errorCodeDescription('provider_unreachable')).toContain('connect to the upstream');
    expect(errorCodeDescription('quota_exceeded')).toContain('quota or allowance');
  });

  it('gracefully formats unknown error codes', () => {
    expect(formatErrorCode('upstream_bad_gateway')).toBe('upstream bad gateway');
    expect(errorCodeDescription('upstream_bad_gateway')).toBe('upstream_bad_gateway');
  });
});

describe('StatusBadge component', () => {
  let app: App | null = null;
  let host: HTMLDivElement | null = null;

  beforeEach(() => {
    host = document.createElement('div');
    document.body.append(host);
  });

  afterEach(() => {
    app?.unmount();
    app = null;
    host?.remove();
    host = null;
  });

  it('renders ok and stopped statuses', () => {
    app = createApp({ render: () => h(StatusBadge, { status: 'ok' }) });
    app.mount(host!);
    expect(host!.textContent).toContain('ok');

    app.unmount();
    host!.textContent = '';
    app = createApp({ render: () => h(StatusBadge, { status: 'aborted' }) });
    app.mount(host!);
    expect(host!.textContent).toContain('stopped');
  });

  it('renders failure badge and formatted error code badge with description tooltip', () => {
    app = createApp({
      render: () => h(StatusBadge, { status: 'error', errorCode: 'provider_rate_limited' }),
    });
    app.mount(host!);

    const badges = host!.querySelectorAll('.oa-badge');
    expect(badges.length).toBe(2);
    expect(badges[0]!.textContent).toBe('failed');
    expect(badges[1]!.textContent).toBe('Rate limited');

    const row = host!.querySelector('.oa-badge-row');
    expect(row?.getAttribute('title')).toContain('rate limiting requests');
  });
});

describe('UsagePanel turns error reason display', () => {
  let app: App | null = null;

  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    app?.unmount();
    app = null;
    document.body.textContent = '';
    vi.clearAllMocks();
    vi.useRealTimers();
  });

  function turn(id: string, status: 'ok' | 'error' | 'aborted', errorCode?: string) {
    return {
      id,
      model_name: `model-${id}`,
      input_tokens: 100,
      output_tokens: 50,
      reasoning_tokens: 0,
      total_tokens: 150,
      credits: 0.05,
      status,
      started_at: Date.now(),
      finished_at: Date.now(),
      ...(errorCode ? { error_code: errorCode } : {}),
    };
  }

  it('displays the error reason and tooltip when a turn errors', async () => {
    vi.mocked(api.get).mockImplementation(async (url: string) => {
      if (url === '/api/usage/me') return { unlimited: true, windows: [] };
      if (url === '/api/usage/cards') return { cards: [] };
      return {
        totals: { requests: 3, input_tokens: 300, output_tokens: 150, reasoning_tokens: 0, total_tokens: 450, credits: 0.15, errors: 1 },
        turns: [
          turn('1', 'ok'),
          turn('2', 'aborted'),
          turn('3', 'error', 'provider_rate_limited'),
        ],
      };
    });

    const panelHost = document.createElement('div');
    const host = document.createElement('div');
    document.body.append(panelHost, host);
    app = createApp({ setup() { providePanelHost(ref(panelHost)); }, render: () => h(UsagePanel) });
    app.mount(host);
    await vi.advanceTimersByTimeAsync(0);
    await nextTick();

    const rows = document.querySelectorAll('.oa-table tbody tr');
    expect(rows.length).toBe(3);

    // Row 1: ok -> empty state cell
    const cell1 = rows[0]!.querySelector('td:last-child');
    expect(cell1?.textContent?.trim()).toBe('');

    // Row 2: aborted -> stopped badge
    const cell2 = rows[1]!.querySelector('td:last-child');
    expect(cell2?.textContent?.trim()).toBe('stopped');

    // Row 3: error with provider_rate_limited -> failed + Rate limited badges with title tooltip
    const cell3 = rows[2]!.querySelector('td:last-child');
    expect(cell3?.textContent).toContain('failed');
    expect(cell3?.textContent).toContain('Rate limited');

    const badgeRow = cell3?.querySelector('.oa-badge-row');
    expect(badgeRow).not.toBeNull();
    expect(badgeRow?.getAttribute('title')).toContain('rate limiting requests');
  });
});
