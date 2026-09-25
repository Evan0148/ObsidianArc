import { api } from './client';

// An account's own invite code — the personal-referral half of the invite
// system. The admin-issued half (batch codes, partner links) lives in the
// backoffice and is not this file's concern: these two routes are the ones a
// signed-in account reaches without an admin permission.

export interface ProfileInvitee {
  nickname: string;
  username: string;
  created_at: number;
  rewarded: boolean;
  /** Why a reward was skipped ('', 'same_ip', 'limit' or 'disabled'). Empty
   *  while the invitee has not yet qualified rather than been refused. */
  reward_skipped: string;
}

export interface ProfileInvites {
  /** Whether accounts get a personal code at all (invites.user_enabled). The
   *  settings section hides itself entirely when this is false. */
  enabled: boolean;
  /** Empty only when `enabled` is false. */
  code: string;
  /** Successful invites this code may still earn a reward for; 0 = unlimited. */
  limit: number;
  used: number;
  reward_cards: number;
  reward_card_days: number;
  invitees: ProfileInvitee[];
}

/**
 * Reads the account's invite standing. The server creates the personal code
 * on this same call the first time it is asked for — under the account's own
 * row lock, so two tabs opened at once cannot mint two codes — which is why
 * there is no separate "create mine" endpoint.
 */
export function fetchProfileInvites(): Promise<ProfileInvites> {
  return api.get<ProfileInvites>('/api/profile/invites');
}

/** Revokes the current personal code and issues a new one in its place. */
export function regenerateProfileInvite(): Promise<ProfileInvites> {
  return api.post<ProfileInvites>('/api/profile/invites/regenerate', {});
}

/**
 * How a code reads on screen: the 8-character alphabet the server generates
 * grouped as XXXX-XXXX, same as it is shown printed or read aloud. A custom
 * partner code (any other length) is shown exactly as stored — splitting an
 * operator's own word into four-character chunks would just be harder to read.
 */
export function formatInviteCode(code: string): string {
  return code.length === 8 ? `${code.slice(0, 4)}-${code.slice(4)}` : code;
}

/** Where a personal or partner code sends a new sign-up. */
export function inviteLink(code: string): string {
  return `${window.location.origin}/register?invite=${encodeURIComponent(code)}`;
}
