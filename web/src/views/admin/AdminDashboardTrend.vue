<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue';
import { ChartNoAxesCombined } from 'lucide-vue-next';
import type { UsagePoint, UsageTotals } from '@/admin/api';
import { currentLanguage, t } from '@/composables/useI18n';
import { compactNumber } from '@/lib/format';

const props = defineProps<{ series: readonly UsagePoint[]; totals: UsageTotals; bucketMs: number }>();
const metric = ref<'requests' | 'total_tokens'>('requests');
const active = ref<number | null>(null);
const focused = ref(false);
const gradientId = `dashboard-area-${useId()}`;
const width = 720;
const height = 174;
const points = computed(() => [...props.series].sort((a, b) => a.at - b.at));
const selected = computed(() => active.value === null ? null : points.value[active.value] ?? null);
const locale = computed(() => currentLanguage() === 'zh' ? 'zh-CN' : 'en-US');
const metricLabel = computed(() => t(metric.value === 'requests' ? 'statRequests' : 'statTokens'));
const hasValues = computed(() => points.value.some((point) => point[metric.value] > 0));
const ceiling = computed(() => {
  const peak = Math.max(4, ...points.value.map((point) => point[metric.value]));
  const magnitude = 10 ** Math.floor(Math.log10(peak));
  return Math.ceil(peak / magnitude) * magnitude;
});
const start = computed(() => points.value[0]?.at ?? 0);
const span = computed(() => (points.value.at(-1)?.at ?? 0) - start.value);
function x(point: UsagePoint): number { return span.value ? (point.at - start.value) / span.value * width : width / 2; }
function y(point: UsagePoint): number { return height - point[metric.value] / ceiling.value * height; }
const line = computed(() => points.value.map((point, i) => `${i ? 'L' : 'M'} ${x(point).toFixed(2)} ${y(point).toFixed(2)}`).join(' '));
const area = computed(() => points.value.length > 1 ? `${line.value} L ${width} ${height} L 0 ${height} Z` : '');
const ticks = computed(() => [1, .75, .5, .25, 0].map((part) => compactNumber(ceiling.value * part)));
const dates = computed(() => {
  const count = Math.min(points.value.length, 7);
  return Array.from({ length: count }, (_, i) => {
    const at = start.value + (count > 1 ? span.value * i / (count - 1) : 0);
    return new Date(at).toLocaleDateString(locale.value, { month: 'numeric', day: 'numeric' });
  });
});
const selectedLabel = computed(() => {
  if (!selected.value) return t('dashboardWeekTotal');
  const date = new Date(selected.value.at).toLocaleString(locale.value, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  const end = new Date(selected.value.at + props.bucketMs).toLocaleTimeString(locale.value, { hour: '2-digit', minute: '2-digit' });
  return `${date} – ${end}`;
});
const value = computed(() => selected.value?.[metric.value] ?? props.totals[metric.value]);
const peak = computed(() => Math.max(0, ...points.value.map((point) => point[metric.value])));
function pointAt(event: PointerEvent): void {
  const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect();
  const at = start.value + Math.max(0, Math.min(1, (event.clientX - bounds.left) / bounds.width)) * span.value;
  active.value = points.value.reduce((closest, point, i) => Math.abs(point.at - at) < Math.abs(points.value[closest]!.at - at) ? i : closest, 0);
}
function move(event: KeyboardEvent): void {
  const last = points.value.length - 1;
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
  event.preventDefault();
  const current = active.value ?? last;
  active.value = event.key === 'Home' ? 0 : event.key === 'End' ? last : Math.max(0, Math.min(last, current + (event.key === 'ArrowRight' ? 1 : -1)));
}
watch(() => props.series, () => { active.value = null; });
watch(metric, () => { active.value = null; });
</script>

<template>
  <section class="oa-dashboard-card oa-dashboard-trend">
    <div class="oa-dashboard-section-head">
      <div><span class="oa-dashboard-kicker">{{ t('secLast7d') }}</span><h2>{{ t('dashboardTrend') }}</h2></div>
      <div class="oa-dashboard-segment" role="group" :aria-label="t('dashboardTrendMetric')">
        <button type="button" :aria-pressed="metric === 'requests'" @click="metric = 'requests'">{{ t('statRequests') }}</button>
        <button type="button" :aria-pressed="metric === 'total_tokens'" @click="metric = 'total_tokens'">{{ t('statTokens') }}</button>
      </div>
    </div>
    <div class="oa-dashboard-trend-summary">
      <div><strong>{{ compactNumber(value) }}</strong><span>{{ metricLabel }}</span></div>
      <span>{{ selectedLabel }}</span>
    </div>
    <div v-if="hasValues" class="oa-dashboard-plot-layout">
      <div class="oa-dashboard-y-axis" aria-hidden="true"><span v-for="(tick, i) in ticks" :key="i">{{ tick }}</span></div>
      <div class="oa-dashboard-plot" tabindex="0" role="slider" :aria-label="t('dashboardChartKeyboard')"
        :aria-valuemin="1" :aria-valuemax="points.length" :aria-valuenow="(active ?? points.length - 1) + 1"
        :aria-valuetext="`${selectedLabel}: ${compactNumber(value)} ${metricLabel}`"
        @pointermove="pointAt" @pointerleave="!focused && (active = null)"
        @focus="focused = true; active = points.length - 1" @blur="focused = false; active = null" @keydown="move">
        <svg :viewBox="`0 -8 ${width} ${height + 16}`" preserveAspectRatio="none" aria-hidden="true">
          <defs><linearGradient :id="gradientId" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stop-color="var(--ai-primary)" stop-opacity=".20" /><stop offset="100%" stop-color="var(--ai-primary)" stop-opacity="0.01" /></linearGradient></defs>
          <line v-for="part in [0, .25, .5, .75, 1]" :key="part" x1="0" :x2="width" :y1="part * height" :y2="part * height" class="oa-dashboard-gridline" />
          <path :d="area" :fill="`url(#${gradientId})`" />
          <path :d="line" class="oa-dashboard-trend-line" vector-effect="non-scaling-stroke" />
          <circle v-if="points.length === 1" :cx="x(points[0]!)" :cy="y(points[0]!)" r="3" fill="var(--ai-primary)" />
          <template v-if="selected"><line :x1="x(selected)" :x2="x(selected)" y1="-8" :y2="height" class="oa-dashboard-crosshair" /><circle :cx="x(selected)" :cy="y(selected)" r="4" class="oa-dashboard-chart-point" /></template>
        </svg>
      </div>
      <div class="oa-dashboard-x-axis" aria-hidden="true"><span v-for="(date, i) in dates" :key="i">{{ date }}</span></div>
    </div>
    <p v-else class="oa-dashboard-empty oa-dashboard-trend-empty"><ChartNoAxesCombined :size="30" aria-hidden="true" />{{ t(metric === 'requests' ? 'noRequestsWeek' : 'dashboardNoTokens') }}</p>
    <div class="oa-dashboard-trend-foot"><span><span class="oa-dashboard-dot" />{{ t('dashboardBucket', { hours: props.bucketMs / 3600000 }) }}</span><span>{{ t('dashboardPeak', { value: compactNumber(peak) }) }}</span></div>
  </section>
</template>
