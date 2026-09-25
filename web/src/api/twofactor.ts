// An account's own two-step verification: what the security settings and the
// setup wizard call. Signing in with a code is in auth.ts, beside login.

import { api } from './client';
import type { Account } from './auth';

export interface TwoFactorStatus {
  available: boolean;
  enabled: boolean;
  enabled_at: number;
  recovery_remaining: number;
  /** Whether the operator's policy covers this account, which is also
   *  whether it may be turned off. */
  mandatory: boolean;
  policy: 'optional' | 'backoffice' | 'admins' | 'everyone';
  remember_days: number;
}

export interface TwoFactorSetup {
  secret: string;
  uri: string;
  issuer: string;
  account: string;
  /** The otpauth link as a QR code, already drawn: the dark modules as one
   *  SVG path in module units, and the side length without the quiet zone. */
  qr: { size: number; path: string };
}

export function fetchTwoFactor(): Promise<TwoFactorStatus> {
  return api.get<TwoFactorStatus>('/api/profile/two-factor');
}

export function beginTwoFactor(): Promise<TwoFactorSetup> {
  return api.post<TwoFactorSetup>('/api/profile/two-factor/setup');
}

export function enableTwoFactor(code: string): Promise<{ recovery_codes: string[]; user: Account }> {
  return api.post<{ recovery_codes: string[]; user: Account }>('/api/profile/two-factor/enable', { code });
}

export function disableTwoFactor(code: string): Promise<{ user: Account }> {
  return api.post<{ user: Account }>('/api/profile/two-factor/disable', { code });
}

export function regenerateRecovery(code: string): Promise<{ recovery_codes: string[] }> {
  return api.post<{ recovery_codes: string[] }>('/api/profile/two-factor/recovery', { code });
}
