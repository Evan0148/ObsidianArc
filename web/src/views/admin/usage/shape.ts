// The shapes the usage charts share, in a module of their own because a
// component's <script setup> can declare them but cannot export them.

import type { UsageBreakdown } from '@/admin/api';

export interface PlotPoint {
  at: number;
  value: number;
}

/** What a board ranks by. `users` is popularity: how many people, not how much. */
export type BoardMetric = 'users' | 'requests' | 'tokens' | 'credits';
export type BoardKind = 'model' | 'user' | 'group' | 'provider';

export function metricOf(row: UsageBreakdown, metric: BoardMetric): number {
  if (metric === 'users') return row.users ?? 0;
  if (metric === 'requests') return row.requests;
  if (metric === 'tokens') return row.total_tokens;
  return row.credits;
}
