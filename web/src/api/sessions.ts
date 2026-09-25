// An account's own signed-in devices: the security settings screen's
// "Signed-in devices" card, over /api/profile/sessions.

import { api } from './client';

export interface DeviceSession {
  /** Opaque — the first sixteen hex characters of the stored session id,
   *  never the session token or the full id. */
  id: string;
  created_at: number;
  last_seen_at: number;
  ip: string;
  user_agent: string;
  /** Whether this is the session the request carrying it arrived on. */
  current: boolean;
}

export function fetchSessions(): Promise<{ sessions: DeviceSession[] }> {
  return api.get<{ sessions: DeviceSession[] }>('/api/profile/sessions');
}

/** Ends one other session. The server refuses this for the caller's own
 *  current one — see revokeOtherSessions for that. */
export function revokeSession(id: string): Promise<void> {
  return api.delete<void>(`/api/profile/sessions/${id}`);
}

/** Ends every session but this one, in a single call. */
export function revokeOtherSessions(): Promise<void> {
  return api.post<void>('/api/profile/sessions/revoke-others');
}
