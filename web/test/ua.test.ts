import { describe, expect, it } from 'vitest';
import { describeUserAgent } from '../src/lib/ua';

const CHROME_WINDOWS =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36';
const CHROME_MAC =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36';
const EDGE_WINDOWS =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36 Edg/119.0.2151.58';
const FIREFOX_LINUX = 'Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/119.0';
const SAFARI_MAC =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15';
const MOBILE_SAFARI_IOS =
  'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1';
const CHROME_IOS =
  'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/119.0.6045.109 Mobile/15E148 Safari/604.1';
const CHROME_ANDROID =
  'Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Mobile Safari/537.36';
const FIREFOX_ANDROID = 'Mozilla/5.0 (Android 13; Mobile; rv:119.0) Gecko/119.0 Firefox/119.0';
const CHROME_CHROMEOS =
  'Mozilla/5.0 (X11; CrOS x86_64 15633.69.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36';

describe('describeUserAgent', () => {
  it('names Chrome on Windows and on macOS', () => {
    expect(describeUserAgent(CHROME_WINDOWS)).toBe('Chrome · Windows');
    expect(describeUserAgent(CHROME_MAC)).toBe('Chrome · macOS');
  });

  it('tells Edge apart from the Chrome it is built on', () => {
    expect(describeUserAgent(EDGE_WINDOWS)).toBe('Edge · Windows');
  });

  it('names Firefox on Linux', () => {
    expect(describeUserAgent(FIREFOX_LINUX)).toBe('Firefox · Linux');
  });

  it('tells Safari apart from Chrome on macOS', () => {
    expect(describeUserAgent(SAFARI_MAC)).toBe('Safari · macOS');
  });

  it('names mobile Safari on iOS', () => {
    expect(describeUserAgent(MOBILE_SAFARI_IOS)).toBe('Safari · iOS');
  });

  it('names Chrome on iOS as Chrome, not Safari, even though it reports CriOS', () => {
    expect(describeUserAgent(CHROME_IOS)).toBe('Chrome · iOS');
  });

  it('names mobile Chrome on Android, not plain Linux', () => {
    expect(describeUserAgent(CHROME_ANDROID)).toBe('Chrome · Android');
  });

  it('names Firefox on Android', () => {
    expect(describeUserAgent(FIREFOX_ANDROID)).toBe('Firefox · Android');
  });

  it('names ChromeOS rather than falling through to Linux', () => {
    expect(describeUserAgent(CHROME_CHROMEOS)).toBe('Chrome · ChromeOS');
  });

  it('falls back to generic words for an agent it cannot place', () => {
    expect(describeUserAgent('curl/8.4.0')).toBe('Unknown browser · Unknown OS');
  });

  it('falls back to "Unknown device" for an empty agent', () => {
    expect(describeUserAgent('')).toBe('Unknown device');
    expect(describeUserAgent(null)).toBe('Unknown device');
    expect(describeUserAgent(undefined)).toBe('Unknown device');
  });
});
