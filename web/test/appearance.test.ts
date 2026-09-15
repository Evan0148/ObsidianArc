import { afterEach, expect, it } from 'vitest';
import { createApp, nextTick, type App } from 'vue';
import AppearanceSection from '../src/views/settings/AppearanceSection.vue';
import { accentPreference, backgroundAccent, setAccentPreference, setBackgroundAccent, setThemeMode, setWallpaper, wallpaper } from '../src/theme/theme';
import { t } from '../src/composables/useI18n';

let app: App | undefined;
afterEach(() => { app?.unmount(); document.body.textContent = ''; localStorage.clear(); });

it('keeps background and control colours independent and clears wallpaper when choosing a background', async () => {
  setThemeMode('light');
  setAccentPreference({ accent: 'blue', customAccent: '' });
  setBackgroundAccent('');
  setWallpaper({ url: '/api/attachments/example', dim: 0, blur: 0, translucency: 0, panelBlur: 0 });
  const host = document.createElement('div'); document.body.appendChild(host);
  app = createApp(AppearanceSection); app.mount(host);
  const primary = document.documentElement.style.getPropertyValue('--ai-primary');
  const mint = [...host.querySelectorAll<HTMLButtonElement>('.oa-background-choice')].find((el) => el.textContent?.includes(t('backgroundMint')))!;
  mint.click(); await nextTick();
  expect(backgroundAccent()).toBe('teal');
  expect(wallpaper()).toBeNull();
  expect(mint.getAttribute('aria-pressed')).toBe('true');
  expect(document.documentElement.style.getPropertyValue('--ai-primary')).toBe(primary);
  const surface = document.documentElement.style.getPropertyValue('--ai-field-bg');
  host.querySelector<HTMLButtonElement>(`.oa-color-dot[aria-label="${t('accentPink')}"]`)!.click();
  await nextTick();
  expect(accentPreference().accent).toBe('pink');
  expect(backgroundAccent()).toBe('teal');
  expect(document.documentElement.style.getPropertyValue('--ai-field-bg')).toBe(surface);
  expect(document.documentElement.style.getPropertyValue('--ai-primary')).not.toBe(primary);
  const cache = JSON.parse(localStorage.getItem('obsidian-arc-palette')!);
  expect(cache.light).toContain(surface);
  expect(host.querySelector('.oa-preview-composer')).toBeNull();
});
