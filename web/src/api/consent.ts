import { api } from './client';

// The consent screen's two calls, and the list of what an account has let in.
//
// The authorisation request itself never passes through here as fields: the
// server hands the screen a signed ticket and takes the same ticket back.
// This module carries it from one call to the other without being able to
// change it, which is the point — a page is somewhere a value can be edited,
// and the two worth editing are the scopes and the callback.

export interface ConsentRequest {
  application: {
    name: string;
    description: string;
    client_id: string;
  };
  /** The claims this application would be given. */
  scopes: string[];
  /** Where the browser will be sent afterwards, whatever the answer. */
  redirect_uri: string;
}

export function fetchConsent(request: string): Promise<ConsentRequest> {
  return api.get<ConsentRequest>(`/api/oauth/consent?request=${encodeURIComponent(request)}`);
}

export function decideConsent(request: string, approve: boolean): Promise<{ redirect: string }> {
  return api.post<{ redirect: string }>('/api/oauth/consent', { request, approve });
}

/** One application this account has let in, as its own settings screen reads it. */
export interface Authorization {
  app_id: string;
  client_id: string;
  name: string;
  scopes: string[];
  created_at: number;
  last_used_at: number;
}

export function fetchAuthorizations(): Promise<{ authorizations: Authorization[] }> {
  return api.get<{ authorizations: Authorization[] }>('/api/oauth/authorizations');
}

/**
 * Withdraws consent and everything issued under it. The application is not
 * merely removed from a list: its tokens stop answering, so it is signed out
 * of this account as well as forgotten by it.
 */
export function withdrawAuthorization(appID: string): Promise<void> {
  return api.delete<void>(`/api/oauth/authorizations/${encodeURIComponent(appID)}`);
}
