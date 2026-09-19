<script setup lang="ts">
// Everything people have told the operator, in one place.
//
// A list of cards rather than a table, because a report is prose: the title
// is the row, but the first line of the body is what tells an operator
// whether to open it, and a table cell that has to hold both makes every
// other column three words wide. The log screen made the same choice for the
// same reason, and this reuses its shape.
//
// Reading one opens the panel beside the list, which is where the whole body
// and the two things an operator can do with it live. Resolving is a status
// and nothing else — the report stays exactly as it was written, because it
// is a record of what somebody said rather than a ticket to edit.

import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { adminApi, type Feedback, type FeedbackStatus, type FeedbackSummary } from '@/admin/api';
import { ApiError } from '@/api/client';
import OaBadge from '@/components/OaBadge.vue';
import OaIconButton from '@/components/OaIconButton.vue';
import OaPagination from '@/components/OaPagination.vue';
import OaPanel from '@/components/OaPanel.vue';
import OaSearchField from '@/components/OaSearchField.vue';
import OaSelectField from '@/components/OaSelectField.vue';
import OaStatGrid from '@/components/OaStatGrid.vue';
import type { Stat } from '@/components/stat';
import type { PageState } from '@/components/table-types';
import { t } from '@/composables/useI18n';
import { IconRefresh } from '@/icons';
import { absoluteTime, relativeTime } from '@/lib/format';
import AdminFailure from './AdminFailure.vue';
import { useAdminView } from './adminView';

const view = useAdminView();
view.setTitle(t('navFeedback'), t('feedbackSubtitle'));

const rows = ref<Feedback[]>([]);
const summary = ref<FeedbackSummary | null>(null);
const total = ref(0);
const pageSize = ref(20);
const offset = ref(0);
const loading = ref(true);
const error = ref('');
const listError = ref('');

const kind = ref('');
const priority = ref('');
const status = ref('');
const search = ref('');

const opened = ref<Feedback | null>(null);
const busy = ref(false);
const panelError = ref('');

/** Newest request wins, so a slow page cannot land on top of a fast one. */
let ticket = 0;

// Typing is the one filter that changes on every keystroke, so it waits for
// the typist to stop rather than asking the server about every letter. The
// three selects below are a click each and go straight through.
let searchTimer = 0;
watch(search, () => {
  window.clearTimeout(searchTimer);
  searchTimer = window.setTimeout(narrow, 250);
});
onBeforeUnmount(() => window.clearTimeout(searchTimer));

const stats = computed<Stat[]>(() => {
  const counts = summary.value;
  if (!counts) return [];
  return [
    { label: t('feedbackStatTotal'), value: String(counts.total) },
    { label: t('feedbackStatOpen'), value: String(counts.open) },
    { label: t('feedbackStatHigh'), value: String(counts.high_open) },
    { label: t('feedbackStatBugs'), value: String(counts.bugs) },
    { label: t('feedbackStatIdeas'), value: String(counts.ideas) },
  ];
});

function query(): string {
  const params = new URLSearchParams();
  if (kind.value) params.set('kind', kind.value);
  if (priority.value) params.set('priority', priority.value);
  if (status.value) params.set('status', status.value);
  if (search.value.trim()) params.set('q', search.value.trim());
  params.set('limit', String(pageSize.value));
  params.set('offset', String(offset.value));
  return `?${params.toString()}`;
}

async function load(): Promise<void> {
  const mine = ++ticket;
  loading.value = true;
  listError.value = '';
  try {
    const data = await adminApi.feedback(query());
    if (mine !== ticket) return;
    rows.value = data.feedback;
    total.value = data.total;
    summary.value = data.summary;
    error.value = '';
    // The row on screen is a copy of one in the list, so a refresh has to
    // hand it the new copy. Kept open when the row is no longer in the list:
    // resolving one while the filter says "open" drops it from the page
    // underneath, and closing the panel out from under the click that did it
    // would read as something having gone wrong.
    if (opened.value) {
      const current = opened.value;
      opened.value = data.feedback.find((row) => row.id === current.id) ?? current;
    }
  } catch (failure) {
    if (mine !== ticket) return;
    const message = failure instanceof ApiError ? failure.message : String(failure);
    // The first load has nothing to keep on screen, so it is the whole page
    // that failed; a later one still has a list worth leaving up.
    if (!summary.value) error.value = message;
    else listError.value = message;
  } finally {
    if (mine === ticket) loading.value = false;
  }
}

/** Any change to what is being asked invalidates which page we are on. */
function narrow(): void {
  offset.value = 0;
  void load();
}

function page(next: PageState): void {
  pageSize.value = next.pageSize;
  offset.value = (next.page - 1) * next.pageSize;
  void load();
}

async function setStatus(next: FeedbackStatus): Promise<void> {
  const record = opened.value;
  if (!record) return;
  busy.value = true;
  panelError.value = '';
  try {
    opened.value = await adminApi.setFeedbackStatus(record.id, next);
    await load();
  } catch (failure) {
    panelError.value = failure instanceof ApiError ? failure.message : String(failure);
  } finally {
    busy.value = false;
  }
}

async function remove(): Promise<void> {
  const record = opened.value;
  if (!record) return;
  busy.value = true;
  panelError.value = '';
  try {
    await adminApi.deleteFeedback(record.id);
    opened.value = null;
    await load();
  } catch (failure) {
    panelError.value = failure instanceof ApiError ? failure.message : String(failure);
  } finally {
    busy.value = false;
  }
}

function kindLabel(row: Feedback): string {
  return t(row.kind === 'bug' ? 'feedbackKindBug' : 'feedbackKindIdea');
}

function priorityLabel(row: Feedback): string {
  switch (row.priority) {
    case 'high': return t('feedbackPriorityHigh');
    case 'low': return t('feedbackPriorityLow');
    default: return t('feedbackPriorityMedium');
  }
}

/** High is the only priority worth colouring: the rest are the normal case. */
function priorityTone(row: Feedback): 'muted' | 'warning' {
  return row.priority === 'high' ? 'warning' : 'muted';
}

function statusLabel(row: Feedback): string {
  return t(row.status === 'resolved' ? 'feedbackStatusResolved' : 'feedbackStatusOpen');
}

/** The first line or so of the body, for the second line of a card. */
function preview(body: string): string {
  const collapsed = body.replace(/\s+/g, ' ').trim();
  return collapsed.length > 110 ? `${collapsed.slice(0, 110)}…` : collapsed;
}

function authorOf(row: Feedback): string {
  return row.nickname || row.username || row.user_id;
}

onMounted(load);
</script>

<template>
  <Teleport :to="view.actionsHost">
    <OaIconButton class="oa-icon-btn" :label="t('refresh')" @click="load">
      <IconRefresh :size="16" />
    </OaIconButton>
  </Teleport>

  <AdminFailure v-if="error" :message="error" @retry="load" />

  <div v-else class="oa-feedback-page">
    <OaStatGrid v-if="summary" id="feedbackSummary" :stats="stats" />

    <div id="feedbackFilters" class="oa-feedback-filters">
      <div class="oa-feedback-filter-grid">
        <OaSelectField
          v-model="status"
          :label="t('colStatus')"
          :searchable="false"
          :options="[
            { value: '', label: t('feedbackAnyStatus') },
            { value: 'open', label: t('feedbackStatusOpen') },
            { value: 'resolved', label: t('feedbackStatusResolved') },
          ]"
          @update:model-value="narrow"
        />
        <OaSelectField
          v-model="kind"
          :label="t('feedbackKind')"
          :searchable="false"
          :options="[
            { value: '', label: t('feedbackAnyKind') },
            { value: 'bug', label: t('feedbackKindBug') },
            { value: 'idea', label: t('feedbackKindIdea') },
          ]"
          @update:model-value="narrow"
        />
        <OaSelectField
          v-model="priority"
          :label="t('feedbackPriority')"
          :searchable="false"
          :options="[
            { value: '', label: t('feedbackAnyPriority') },
            { value: 'high', label: t('feedbackPriorityHigh') },
            { value: 'medium', label: t('feedbackPriorityMedium') },
            { value: 'low', label: t('feedbackPriorityLow') },
          ]"
          @update:model-value="narrow"
        />
        <OaSearchField v-model="search" :label="t('feedbackSearch')" />
      </div>
    </div>

    <div id="feedbackList" class="oa-feedback-results">
      <p v-if="loading" class="oa-menu-empty">{{ t('loading') }}</p>
      <p v-else-if="listError" class="oa-menu-empty">{{ listError }}</p>
      <p v-else-if="!rows.length" class="oa-menu-empty">{{ t('feedbackEmpty') }}</p>

      <div v-else class="oa-feedback-cards">
        <button
          v-for="(row, index) in rows"
          :key="row.id"
          type="button"
          class="oa-feedback-card"
          :class="{ resolved: row.status === 'resolved', selected: opened?.id === row.id }"
          :style="{ '--item-idx': String(index) }"
          @click="opened = row"
        >
          <span class="oa-feedback-card-rail" :class="row.kind" aria-hidden="true" />
          <span class="oa-feedback-card-main">
            <span class="oa-feedback-card-head">
              <span class="oa-feedback-card-title">{{ row.title }}</span>
              <OaBadge :tone="row.status === 'resolved' ? 'muted' : 'default'">
                {{ statusLabel(row) }}
              </OaBadge>
            </span>
            <span class="oa-feedback-card-body">{{ preview(row.body) }}</span>
            <span class="oa-feedback-card-meta">
              <OaBadge tone="muted">{{ kindLabel(row) }}</OaBadge>
              <OaBadge :tone="priorityTone(row)">{{ priorityLabel(row) }}</OaBadge>
              <span>{{ t('feedbackFrom', { name: authorOf(row) }) }}</span>
              <span>{{ relativeTime(row.created_at) }}</span>
            </span>
          </span>
        </button>
      </div>

      <OaPagination
        :page="Math.floor(offset / pageSize) + 1"
        :page-size="pageSize"
        :total="total"
        :busy="loading"
        @change="page"
      />
    </div>
  </div>

  <OaPanel
    v-if="opened"
    :title="opened.title"
    :confirm-label="opened.status === 'resolved' ? t('feedbackReopen') : t('feedbackResolve')"
    :cancel-label="t('close')"
    :destructive-label="t('deleteLabel')"
    :destructive-confirm="t('feedbackDeleteConfirm')"
    :width="520"
    :busy="busy"
    :error="panelError"
    @close="opened = null"
    @confirm="setStatus(opened.status === 'resolved' ? 'open' : 'resolved')"
    @destructive="remove"
  >
    <div class="oa-feedback-detail-badges">
      <OaBadge tone="muted">{{ kindLabel(opened) }}</OaBadge>
      <OaBadge :tone="priorityTone(opened)">{{ priorityLabel(opened) }}</OaBadge>
      <OaBadge :tone="opened.status === 'resolved' ? 'muted' : 'default'">{{ statusLabel(opened) }}</OaBadge>
    </div>

    <dl class="oa-log-facts">
      <dt>{{ t('colUser') }}</dt>
      <dd>{{ authorOf(opened) }}</dd>
      <dt>{{ t('colCreated') }}</dt>
      <dd>{{ absoluteTime(opened.created_at) }}</dd>
      <dt>{{ t('colUpdated') }}</dt>
      <dd>{{ relativeTime(opened.updated_at) }}</dd>
    </dl>

    <!-- Interpolated, never v-html: this is text somebody else typed, and the
         one thing it must never be is markup. -->
    <p class="oa-feedback-detail-body">{{ opened.body }}</p>
  </OaPanel>
</template>
