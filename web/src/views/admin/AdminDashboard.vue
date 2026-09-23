<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { ArrowUpRight, ArrowDownLeft, ArrowUpLeft, ChartNoAxesCombined, CircleAlert, Coins, RefreshCw, CalendarDays } from 'lucide-vue-next';
import { adminApi, type Dashboard, type UsageBreakdown } from '@/admin/api';
import OaChart from '@/components/OaChart.vue';
import OaCellStack from '@/components/OaCellStack.vue';
import OaIconButton from '@/components/OaIconButton.vue';
import { currentLanguage, t } from '@/composables/useI18n';
import { IconSpark, IconUsers, IconServer } from '@/icons';
import { compactNumber, relativeTime, tokenFigure } from '@/lib/format';
import { fold, type ChartShape } from '@/lib/chart';
import { canAdmin } from '@/stores/session';
import AdminDashboardTrend from './AdminDashboardTrend.vue';
import AdminFailure from './AdminFailure.vue';
import StatusBadge from './StatusBadge.vue';
import { useAdminView } from './adminView';

const view = useAdminView();
view.setTitle(t('navDashboard'));
const data = ref<Dashboard | null>(null);
const error = ref('');
const busy = ref(false);
const updatedAt = ref(0);
const ranking = ref<'models' | 'users'>('models');
const shape = ref<ChartShape>(dashboardShape);
const locale = computed(() => currentLanguage() === 'zh' ? 'zh-CN' : 'en-US');
const dateLabel = computed(() => new Date(updatedAt.value || Date.now()).toLocaleDateString(locale.value, {
  month: 'long', day: 'numeric', weekday: 'long',
}));
const updatedLabel = computed(() => updatedAt.value ? t('dashboardUpdated', {
  time: new Date(updatedAt.value).toLocaleTimeString(locale.value, { hour: '2-digit', minute: '2-digit' }),
}) : '');
const failureRate = computed(() => {
  const totals = data.value?.last_24h;
  return totals?.requests ? `${(totals.errors / totals.requests * 100).toFixed(1)}%` : '—';
});
const resources = computed(() => {
  const counts = data.value?.counts;
  if (!counts) return [];
  return [
    { key: 'users', icon: IconUsers, label: t('statUsers'), value: counts.users, note: t('nActive', { count: counts.active_users }) },
    { key: 'providers', icon: IconServer, label: t('statProviders'), value: counts.providers, note: t('nEnabled', { count: counts.enabled_providers }) },
    { key: 'models', icon: IconSpark, label: t('statModels'), value: counts.models, note: t('nEnabled', { count: counts.enabled_models }) },
  ];
});
const rankedRows = computed(() => ranking.value === 'models' ? data.value?.top_models ?? [] : data.value?.top_users ?? []);
// Ranked by tokens, not credits. Credits are tokens times a price the
// operator set per model, so a free model — weight 0 — carrying half the
// traffic vanished from this chart entirely, and a pricey one looked busier
// than it was. "Where the load goes" is a question about tokens; what it
// cost is still in the breakdown table underneath.
const rankedSlices = computed(() => fold(rankedRows.value.map((row) => ({
  key: row.key, label: row.label || row.key || '—', value: row.total_tokens,
})), 5, t('chartOther')));
const rankedTotal = computed(() => rankedSlices.value.reduce((sum, row) => sum + row.value, 0));
function share(value: number): string {
  return rankedTotal.value ? `${(value / rankedTotal.value * 100).toFixed(1)}%` : '0%';
}
function rowNote(key: string): string {
  const row = rankedRows.value.find((entry) => entry.key === key);
  return row ? t('dashboardRankNote', { requests: compactNumber(row.requests), credits: compactNumber(row.credits) }) : '';
}
function exactCredits(row: UsageBreakdown): string { return row.credits.toLocaleString(locale.value, { maximumFractionDigits: 2 }); }
function onShape(next: ChartShape): void { shape.value = next; dashboardShape = next; }

async function load(): Promise<void> {
  if (busy.value) return;
  busy.value = true;
  error.value = '';
  try {
    // The server orders the breakdown table; asking for the same metric the
    // chart draws keeps the two in the same order.
    data.value = await adminApi.dashboard('tokens');
    updatedAt.value = Date.now();
  } catch (failure) {
    error.value = failure instanceof Error ? failure.message : String(failure);
  } finally {
    busy.value = false;
  }
}
onMounted(load);
</script>

<script lang="ts">
// Preserve the existing chart preference when the administrator returns.
let dashboardShape: ChartShape = 'bar';
</script>

<template>
  <AdminFailure v-if="error" :message="error" @retry="load" />
  <div v-if="!data && busy" class="oa-dashboard-loading" role="status" :aria-label="t('loading')">
    <span>{{ t('loading') }}</span><div /><div /><div />
  </div>

  <div v-if="data" class="oa-dashboard" :aria-busy="busy">
    <header class="oa-dashboard-intro">
      <div>
        <h1>{{ t('dashboardHeading') }}</h1>
        <p v-if="t('dashboardIntro')">{{ t('dashboardIntro') }}</p>
      </div>
      <div class="oa-dashboard-intro-meta">
        <span class="oa-dashboard-date"><CalendarDays :size="14" aria-hidden="true" />{{ dateLabel }}</span>
        <div><span class="oa-dashboard-updated" role="status">{{ updatedLabel }}</span>
          <OaIconButton class="oa-icon-btn oa-dashboard-refresh" :label="t('refresh')" :disabled="busy" @click="load">
            <RefreshCw :size="14" :class="{ 'is-refreshing': busy }" aria-hidden="true" />
          </OaIconButton>
        </div>
      </div>
    </header>

    <section class="oa-dashboard-metrics" :aria-label="t('secLast24h')">
      <article class="oa-dashboard-metric oa-dashboard-metric-featured">
        <div class="oa-dashboard-metric-top"><span>{{ t('statRequests') }}</span><ChartNoAxesCombined :size="19" aria-hidden="true" /></div>
        <strong :title="data.last_24h.requests.toLocaleString(locale)">{{ compactNumber(data.last_24h.requests) }}</strong>
        <div class="oa-dashboard-metric-note"><span class="oa-dashboard-dot" />{{ t('secLast24h') }}</div>
        <svg class="oa-dashboard-metric-orbit" viewBox="0 0 160 160" aria-hidden="true"><circle cx="132" cy="132" r="38" /><circle cx="132" cy="132" r="66" /><circle cx="132" cy="132" r="94" /></svg>
      </article>
      <article class="oa-dashboard-metric">
        <div class="oa-dashboard-metric-top"><span>{{ t('statTokens') }}</span><IconSpark :size="19" /></div>
        <strong :title="data.last_24h.total_tokens.toLocaleString(locale)">{{ compactNumber(data.last_24h.total_tokens) }}</strong>
        <div class="oa-dashboard-metric-note oa-dashboard-token-split">
          <span><ArrowDownLeft :size="12" aria-hidden="true" />{{ t('dashboardInput') }} {{ compactNumber(data.last_24h.input_tokens) }}</span>
          <span><ArrowUpLeft :size="12" aria-hidden="true" />{{ t('dashboardOutput') }} {{ compactNumber(data.last_24h.output_tokens) }}</span>
        </div>
      </article>
      <article class="oa-dashboard-metric">
        <div class="oa-dashboard-metric-top"><span>{{ t('statCredits') }}</span><Coins :size="19" aria-hidden="true" /></div>
        <strong :title="data.last_24h.credits.toLocaleString(locale)">{{ compactNumber(data.last_24h.credits) }}</strong>
        <div class="oa-dashboard-metric-note">{{ t('dashboardWeeklyCredits', { value: compactNumber(data.last_7d.credits) }) }}</div>
      </article>
      <article class="oa-dashboard-metric" :class="{ 'has-errors': data.last_24h.errors > 0 }">
        <div class="oa-dashboard-metric-top"><span>{{ t('dashboardFailedRequests') }}</span><CircleAlert :size="19" aria-hidden="true" /></div>
        <strong :title="data.last_24h.errors.toLocaleString(locale)">{{ compactNumber(data.last_24h.errors) }}</strong>
        <div class="oa-dashboard-metric-note">{{ data.last_24h.requests ? t('dashboardFailureRate', { rate: failureRate }) : t('noRequestsYet') }}</div>
      </article>
    </section>

    <section id="secInstance" class="oa-dashboard-resources" :aria-label="t('secInstance')">
      <div class="oa-dashboard-resources-label"><span class="oa-dashboard-dot" />{{ t('dashboardWorkspace') }}</div>
      <component :is="canAdmin(resource.key) ? 'RouterLink' : 'div'" v-for="resource in resources" :key="resource.key"
        :to="canAdmin(resource.key) ? `/admin/${resource.key}` : undefined" class="oa-dashboard-resource">
        <span class="oa-dashboard-resource-icon"><component :is="resource.icon" :size="17" /></span>
        <span class="oa-dashboard-resource-copy"><span>{{ resource.label }} <strong>{{ resource.value.toLocaleString(locale) }}</strong></span><small>{{ resource.note }}</small></span>
        <ArrowUpRight v-if="canAdmin(resource.key)" :size="14" aria-hidden="true" />
      </component>
    </section>

    <div class="oa-dashboard-middle">
      <AdminDashboardTrend :series="data.series" :totals="data.last_7d" :bucket-ms="data.bucket_ms" />

      <section id="secBusiestModels" class="oa-dashboard-card oa-dashboard-ranking">
        <div class="oa-dashboard-section-head">
          <div><span class="oa-dashboard-kicker">{{ t('secLast7d') }}</span><h2>{{ t('dashboardRanking') }}</h2></div>
          <div class="oa-dashboard-segment" :aria-label="t('secRanking')" role="group">
            <button type="button" :aria-pressed="ranking === 'models'" @click="ranking = 'models'">{{ t('statModels') }}</button>
            <button type="button" :aria-pressed="ranking === 'users'" @click="ranking = 'users'">{{ t('statUsers') }}</button>
          </div>
        </div>
        <div class="oa-dashboard-rank-caption">
          <span>{{ t('dashboardRankHint') }}</span>
          <div class="oa-dashboard-shapes" :aria-label="t('chartShape')" role="group">
            <button type="button" :aria-pressed="shape === 'bar'" @click="onShape('bar')">{{ t('chartBar') }}</button>
            <button type="button" :aria-pressed="shape === 'pie'" @click="onShape('pie')">{{ t('chartPie') }}</button>
          </div>
        </div>
        <p v-if="!rankedSlices.length" class="oa-dashboard-empty"><IconSpark :size="26" />{{ t('nothingYet') }}</p>
        <ol v-else-if="shape === 'bar'" class="oa-dashboard-rank-list">
          <li v-for="(row, index) in rankedSlices" :key="row.key || 'other'" :title="`${row.label} · ${rowNote(row.key)}`">
            <span class="oa-dashboard-rank-number">{{ String(index + 1).padStart(2, '0') }}</span>
            <div class="oa-dashboard-rank-main">
              <div class="oa-dashboard-rank-label"><span>{{ row.label }}</span><strong :title="row.value.toLocaleString(locale)">{{ compactNumber(row.value) }}</strong></div>
              <div class="oa-dashboard-rank-bottom"><span class="oa-dashboard-rank-track"><span :style="{ width: share(row.value) }" /></span><small>{{ share(row.value) }}</small></div>
            </div>
          </li>
        </ol>
        <OaChart v-else class="oa-dashboard-pie" shape="pie" :data="rankedSlices" :format="compactNumber" :empty-text="t('nothingYet')" />
        <details v-if="rankedRows.length" class="oa-dashboard-ranking-details">
          <summary>{{ t('dashboardRankDetails') }}</summary>
          <div class="oa-dashboard-table-wrap" tabindex="0" :aria-label="t('dashboardRankDetails')">
            <table class="oa-dashboard-table">
              <thead><tr><th>{{ ranking === 'models' ? t('colModel') : t('colUser') }}</th><th>{{ t('colRequests') }}</th><th>{{ t('colTokens') }}</th><th>{{ t('colCredits') }}</th></tr></thead>
              <tbody><tr v-for="row in rankedRows" :key="row.key"><td>{{ row.label || row.key || '—' }}</td><td>{{ compactNumber(row.requests) }}</td><td>{{ compactNumber(row.total_tokens) }}</td><td>{{ exactCredits(row) }}</td></tr></tbody>
            </table>
          </div>
        </details>
      </section>
    </div>

    <section id="secRecentRequests" class="oa-dashboard-card oa-dashboard-recent">
      <div class="oa-dashboard-section-head">
        <div><h2>{{ t('secRecentRequests') }}</h2><p>{{ t('dashboardRecentHint') }}</p></div>
        <RouterLink v-if="canAdmin('usage')" to="/admin/usage" class="oa-dashboard-link">{{ t('dashboardViewUsage') }}<ArrowUpRight :size="14" aria-hidden="true" /></RouterLink>
      </div>
      <p v-if="!data.recent.length" class="oa-dashboard-empty"><ChartNoAxesCombined :size="26" aria-hidden="true" />{{ t('noRequestsYet') }}</p>
      <div v-else class="oa-dashboard-table-wrap" tabindex="0" :aria-label="t('secRecentRequests')">
        <table class="oa-dashboard-table oa-dashboard-recent-table">
          <thead><tr><th>{{ t('colUser') }}</th><th>{{ t('colModel') }}</th><th>{{ t('colTokens') }}</th><th>{{ t('colStatus') }}</th><th>{{ t('colWhen') }}</th></tr></thead>
          <tbody>
            <tr v-for="row in data.recent" :key="row.id">
              <td><span class="oa-dashboard-user"><span class="oa-dashboard-avatar" aria-hidden="true">{{ (row.username || row.user_id).slice(0, 1).toLocaleUpperCase() }}</span><span>{{ row.username || row.user_id }}</span></span></td>
              <td><OaCellStack :title="row.model_name || '—'" :sub="row.provider_name" /></td>
              <td class="oa-dashboard-numeric" :title="row.estimated ? t('tokensEstimatedHint') : row.total_tokens.toLocaleString(locale)">{{ tokenFigure(row.total_tokens, row.estimated) }}</td>
              <td><StatusBadge :status="row.status" :error-code="row.error_code" /></td>
              <td class="oa-dashboard-when" :title="new Date(row.started_at).toLocaleString(locale)">{{ relativeTime(row.started_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
    <footer class="oa-dashboard-foot"><span class="oa-dashboard-dot" />{{ t('dashboardFootnote') }}</footer>
  </div>
</template>
