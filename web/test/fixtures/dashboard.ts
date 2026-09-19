import type { Dashboard, UsageTotals } from '../../src/admin/api';

export const emptyTotals: UsageTotals = {
  requests: 0, input_tokens: 0, output_tokens: 0, reasoning_tokens: 0,
  total_tokens: 0, credits: 0, errors: 0,
};

export function dashboardFixture(): Dashboard {
  const at = Date.UTC(2026, 8, 12, 6);
  const series = [120, 180, 96, 80, 240, 390, 210, 165, 290, 430, 310, 210, 260, 550, 480, 310, 420, 650, 460, 330, 590, 830, 580, 460, 720, 990, 760, 610].map((requests, i) => ({
    ...emptyTotals, at: at + i * 21600000, requests,
    input_tokens: requests * 12000, output_tokens: requests * 150,
    total_tokens: requests * 12150, credits: requests * 310,
  }));
  return {
    counts: { users: 176, active_users: 165, providers: 11, enabled_providers: 11, models: 40, enabled_models: 40 },
    newest_users: [],
    last_24h: { ...emptyTotals, requests: 4906, input_tokens: 166700000, output_tokens: 1900000, total_tokens: 168600000, credits: 2000000, errors: 1071 },
    last_7d: series.reduce((totals, point) => ({ ...totals, requests: totals.requests + point.requests, input_tokens: totals.input_tokens + point.input_tokens, output_tokens: totals.output_tokens + point.output_tokens, total_tokens: totals.total_tokens + point.total_tokens, credits: totals.credits + point.credits }), { ...emptyTotals }),
    top_models: ['Claude Sonnet 4.5', 'GPT-5', 'Gemini 2.5 Pro', 'DeepSeek V3', 'Qwen3', 'Other model'].map((label, i) => ({
      ...emptyTotals, key: `model-${i}`, label, requests: 1500 - i * 230, total_tokens: 12000000 - i * 1900000, credits: [840000, 640000, 340000, 126000, 64000, 42000][i]!,
    })),
    top_users: ['onyx', 'lin', 'momo', 'alex', 'ada'].map((label, i) => ({
      ...emptyTotals, key: `user-${i}`, label, requests: 1200 - i * 200, total_tokens: 9000000 - i * 1000000, credits: 450000 - i * 70000,
    })),
    series, bucket_ms: 21600000,
    recent: ['onyx', 'lin', 'momo', 'alex', 'ada', 'onyx', 'lin', 'momo'].map((username, i) => ({
      ...emptyTotals, id: `request-${i}`, user_id: `user-${i}`, username,
      model_name: ['Claude Sonnet 4.5', 'GPT-5', 'Gemini 2.5 Pro'][i % 3]!, provider_name: ['Anthropic', 'OpenAI', 'Google'][i % 3]!,
      conversation_id: '', total_tokens: 2400 + i * 760, status: i === 2 ? 'error' : 'ok', error_code: '',
      started_at: Date.now() - (i * 12 + 3) * 60000, duration_ms: 1300,
    })),
  };
}
