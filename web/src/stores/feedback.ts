// Whether this account has an answer waiting on one of its reports.
//
// One number, shared: the account menu draws a dot from it on every page, and
// the feedback panel clears it the moment a thread is opened. Holding it here
// rather than in either of them is what keeps the dot from surviving the read
// that should have cleared it — the panel and the menu are not on screen at
// the same time, so neither can tell the other.
//
// Plain refs and functions, like the other two stores here: a count and two
// actions do not need a store library.

import { ref, type Ref } from 'vue';
import { fetchFeedbackUnread } from '@/api/feedback';

const unread = ref(0);

export const feedbackUnread: Ref<number> = unread;

/**
 * Asks the server. Failures are swallowed: a dot nobody can draw is not worth
 * an error state, and the panel behind it works either way.
 */
export async function refreshFeedbackUnread(): Promise<void> {
  try {
    const { unread: count } = await fetchFeedbackUnread();
    unread.value = count;
  } catch {
    // Left at whatever it was. A stale dot is a smaller lie than a wrong one.
  }
}

/** Set directly when a screen has just learned the answer for itself. */
export function setFeedbackUnread(count: number): void {
  unread.value = Math.max(0, count);
}

/** Signed out: the next account's dot is not this one's. */
export function forgetFeedbackUnread(): void {
  unread.value = 0;
}
