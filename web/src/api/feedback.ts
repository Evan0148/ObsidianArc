// Feedback, as the person sending it sees it.
//
// The enums are the server's own values, English on purpose like every other
// protocol string in this project: the screen translates them at render, and
// what travels is what the API documents.

import { api } from './client';

export type FeedbackKind = 'bug' | 'idea';
export type FeedbackPriority = 'low' | 'medium' | 'high';
export type FeedbackStatus = 'open' | 'resolved';

export interface Feedback {
  id: string;
  user_id: string;
  kind: FeedbackKind;
  priority: FeedbackPriority;
  status: FeedbackStatus;
  title: string;
  body: string;
  created_at: number;
  updated_at: number;
  /**
   * Who wrote it. Filled only in the operator's listing — an author reading
   * their own knows who they are, and the join is not free.
   */
  username?: string;
  nickname?: string;
}

export interface FeedbackList {
  feedback: Feedback[];
  /** How many more this account may send today. */
  remaining: number;
  max_per_day: number;
}

export function listFeedback(): Promise<FeedbackList> {
  return api.get<FeedbackList>('/api/feedback');
}

export function sendFeedback(body: {
  kind: FeedbackKind;
  priority: FeedbackPriority;
  title: string;
  body: string;
  /** The challenge token, where the operator has switched one on. */
  turnstile?: string;
}): Promise<Feedback> {
  return api.post<Feedback>('/api/feedback', body);
}
