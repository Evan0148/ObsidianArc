// Maps machine error codes stored on usage records and turns to localized,
// human-readable labels and descriptions for table rows and badges.

import { t } from '@/composables/useI18n';
import type { StringKey } from '@/i18n';

const KNOWN_CODES = new Set([
  'provider_rate_limited',
  'provider_unreachable',
  'provider_auth',
  'provider_error',
  'provider_rejected',
  'refusal',
  'images_unsupported',
  'internal',
  'cancelled',
  'rate_limited',
  'quota_exceeded',
  'context_length_exceeded',
  'timeout',
  'invalid_request',
  'moderation',
]);

/** Returns a concise label for badges and table cells (e.g. "Rate limited" / "供应商限流"). */
export function formatErrorCode(code: string | undefined): string {
  if (!code) return '';
  if (KNOWN_CODES.has(code)) {
    return t(`err_${code}` as StringKey);
  }
  return code.replace(/_/g, ' ');
}

/** Returns a user-friendly explanatory sentence for tooltips and hover titles. */
export function errorCodeDescription(code: string | undefined): string {
  if (!code) return '';
  if (KNOWN_CODES.has(code)) {
    return t(`err_desc_${code}` as StringKey);
  }
  return code;
}
