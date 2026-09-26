<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue';
import { useResizeObserver } from '@vueuse/core';

const props = withDefaults(defineProps<{
  text: string;
  speed?: number;
  gap?: number;
  pauseOnHover?: boolean;
}>(), {
  speed: 28,
  gap: 28,
  pauseOnHover: true,
});

const containerRef = ref<HTMLElement | null>(null);
const sizerRef = ref<HTMLElement | null>(null);

const isOverflowing = ref(false);
const duration = ref(8);

function checkOverflow(): void {
  const container = containerRef.value;
  const sizer = sizerRef.value;
  if (!container || !sizer) return;

  const containerWidth = container.clientWidth;
  const contentWidth = sizer.scrollWidth;

  if (containerWidth <= 0 || contentWidth <= 0) {
    isOverflowing.value = false;
    return;
  }

  if (contentWidth > containerWidth + 1) {
    isOverflowing.value = true;
    const totalDistance = contentWidth + props.gap;
    // 76% of time is moving at speed, 24% is pausing (12% start + 12% end)
    const moveSeconds = totalDistance / props.speed;
    const totalSeconds = moveSeconds / 0.76;
    duration.value = Math.max(3, Math.round(totalSeconds * 10) / 10);
  } else {
    isOverflowing.value = false;
  }
}

useResizeObserver(containerRef, checkOverflow);

watch(() => props.text, () => {
  void nextTick(checkOverflow);
});

onMounted(() => {
  void nextTick(checkOverflow);
});

defineExpose({ checkOverflow });
</script>

<template>
  <span
    ref="containerRef"
    class="oa-marquee"
    :class="{ 'is-overflowing': isOverflowing, 'pause-on-hover': pauseOnHover }"
    :style="{
      '--marquee-duration': `${duration}s`,
    }"
    :title="text"
  >
    <!-- Invisible sizer: layout-independent measurement of true text width -->
    <span ref="sizerRef" class="oa-marquee-sizer" aria-hidden="true">{{ text }}</span>

    <!-- When text fits comfortably without truncation -->
    <span v-if="!isOverflowing" class="oa-marquee-static">
      {{ text }}
    </span>

    <!-- When text exceeds available container width: seamless marquee loop -->
    <span v-else class="oa-marquee-track">
      <span class="oa-marquee-slice">
        <span class="oa-marquee-item">{{ text }}</span>
        <span class="oa-marquee-spacer" aria-hidden="true" :style="{ width: `${gap}px` }" />
      </span>
      <span class="oa-marquee-slice" aria-hidden="true">
        <span class="oa-marquee-item">{{ text }}</span>
        <span class="oa-marquee-spacer" aria-hidden="true" :style="{ width: `${gap}px` }" />
      </span>
    </span>
  </span>
</template>
