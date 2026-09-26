import { computed } from 'vue';
import { useMediaQuery } from '@vueuse/core';
import { useTheme } from '@/composables/useTheme';
import { siteInfo } from '@/stores/session';

export function useLoginBackground() {
  const theme = useTheme();
  const isPortrait = useMediaQuery('(orientation: portrait)');
  const site = computed(() => siteInfo.value);

  const loginBgUrl = computed(() => {
    const bg = site.value.login_background;
    if (!bg) return '';
    const dark = theme.dark();
    const portrait = isPortrait.value;

    // Prioritized fallback cascade:
    // 1. Exact orientation and exact color scheme
    // 2. Opposite orientation keeping color scheme (maintains brightness/contrast)
    // 3. Exact orientation with opposite color scheme
    // 4. Any remaining available variant
    if (portrait) {
      if (dark) {
        return bg.portrait_dark || bg.landscape_dark || bg.portrait_light || bg.landscape_light || '';
      }
      return bg.portrait_light || bg.landscape_light || bg.portrait_dark || bg.landscape_dark || '';
    }
    if (dark) {
      return bg.landscape_dark || bg.portrait_dark || bg.landscape_light || bg.portrait_light || '';
    }
    return bg.landscape_light || bg.portrait_light || bg.landscape_dark || bg.portrait_dark || '';
  });

  return {
    loginBgUrl,
    isPortrait,
  };
}
