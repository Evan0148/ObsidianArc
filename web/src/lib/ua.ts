// A tiny user-agent → "Chrome · macOS" describer, for the signed-in-devices
// list. No dependency: what that row needs is a handful of substring checks,
// not a parser, and a library here would be a second dependency just to draw
// a line of text nobody clicks through.
//
// Order matters more than any individual check. Edge's own user agent still
// carries "Chrome", Chrome on iOS identifies itself as "CriOS" while still
// carrying "Safari", and every mobile Chrome or mobile Safari carries the
// same desktop tokens beside "Mobile" — so the more specific brand has to be
// tested before the one it is built on, in both lists below.

import { t } from '@/composables/useI18n';

function detectBrowser(ua: string): string {
  if (/Edg(?:e|A|iOS)?\//.test(ua)) return 'Edge';
  if (/OPR\/|Opera/.test(ua)) return 'Opera';
  if (/Firefox\/|FxiOS\//.test(ua)) return 'Firefox';
  // Chrome on iOS reports itself as CriOS, not Chrome, and both still carry
  // "Safari" further along the string — so this has to come before Safari's
  // own check, the same way Edge's check comes before this one.
  if (/CriOS\/|Chrome\/|Chromium\//.test(ua)) return 'Chrome';
  // Real Safari carries "Version/", which no Chromium browser's user agent
  // does even though every one of them also carries "Safari/" from the
  // rendering engine they are built on.
  if (/Safari\//.test(ua) && /Version\//.test(ua)) return 'Safari';
  if (/MSIE |Trident\//.test(ua)) return 'Internet Explorer';
  return '';
}

function detectOS(ua: string): string {
  if (/Windows NT/.test(ua)) return 'Windows';
  // iOS's own user agent contains "like Mac OS X", so this has to be checked
  // before the desktop Mac OS X test below.
  if (/iPhone|iPad|iPod/.test(ua)) return 'iOS';
  // Android's user agent opens with "Linux; Android …", so this has to be
  // checked before the plain Linux test below.
  if (/Android/.test(ua)) return 'Android';
  if (/Mac OS X/.test(ua)) return 'macOS';
  if (/CrOS/.test(ua)) return 'ChromeOS';
  if (/Linux/.test(ua)) return 'Linux';
  return '';
}

/**
 * Describes a browser's user agent the way the signed-in-devices list shows
 * it — "Chrome · macOS", "Safari · iOS", "Firefox · Linux". Either half
 * falls back to a generic word rather than vanishing, so a row from a client
 * this cannot place is still a row rather than a blank.
 */
export function describeUserAgent(ua: string | null | undefined): string {
  const value = (ua ?? '').trim();
  if (!value) return t('uaUnknownDevice');
  const browser = detectBrowser(value) || t('uaUnknownBrowser');
  const os = detectOS(value) || t('uaUnknownOS');
  return `${browser} · ${os}`;
}
