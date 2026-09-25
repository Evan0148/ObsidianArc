// The bell's own feed: what happened to this account, or to the instance,
// while it was not looking.
//
// A row carries a kind and a bag of params, never a sentence — see
// lib/notification-text.ts for where kind + params become English or
// Chinese. The server is never in the business of composing either.

import { api } from './client';

export interface Notification {
  id: string;
  kind: string;
  params: Record<string, unknown>;
  link: string;
  created_at: number;
}

export interface NotificationFeed {
  notifications: Notification[];
  unread: number;
  seen_at: number;
  /** The server's own clock, epoch ms — the cursor starts here rather than at
   *  the newest row's own timestamp, or an account whose visibility widens
   *  later (promoted to admin) would see its whole backlog as new toasts. */
  now: number;
}

export interface NotificationPoll {
  notifications: Notification[];
  unread: number;
  now: number;
}

export function fetchNotifications(before?: number): Promise<NotificationFeed> {
  const query = before ? `?before=${before}` : '';
  return api.get<NotificationFeed>(`/api/notifications${query}`);
}

/** after is the last moment already seen — a millisecond timestamp, or 0 for
 *  the beginning of time. The server re-returns anything within a 10s grace
 *  window of it, so the caller must de-duplicate by id. */
export function pollNotifications(after: number): Promise<NotificationPoll> {
  return api.get<NotificationPoll>(`/api/notifications/poll?after=${after}`);
}

/** Moves the read watermark to min(upTo, server now), or to server now when
 *  omitted — never backwards, either way. */
export function markNotificationsRead(upTo?: number): Promise<void> {
  return api.post<void>('/api/notifications/read', upTo ? { up_to: upTo } : {});
}
