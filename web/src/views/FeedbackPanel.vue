<script setup lang="ts">
// Where somebody tells the operator that something is broken, or that it
// could be better.
//
// A column beside the chat like every other panel here, and the form is the
// whole of it: two questions with two or three answers each, a title, and a
// box big enough that a real description does not feel unwelcome. The
// segmented controls are there rather than selects because every option fits
// on screen — a menu that has to be opened to show two choices is a menu that
// exists to hide one of them.
//
// Below the form is what this account has already sent, with the status an
// administrator has put on it. That list is the reason this is not a mail
// form: it is the only way somebody can see that their report arrived and
// what came of it, which is also what stops the same report arriving four
// times.

import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { ApiError } from '@/api/client';
import {
  listFeedback, sendFeedback,
  type Feedback, type FeedbackKind, type FeedbackPriority,
} from '@/api/feedback';
import OaBadge from '@/components/OaBadge.vue';
import OaPanel from '@/components/OaPanel.vue';
import OaTextArea from '@/components/OaTextArea.vue';
import OaTextField from '@/components/OaTextField.vue';
import OaTurnstile from '@/components/OaTurnstile.vue';
import { t, type StringKey } from '@/composables/useI18n';
import { relativeTime } from '@/lib/format';
import { siteInfo } from '@/stores/session';

const KINDS: Array<{ value: FeedbackKind; label: StringKey }> = [
  { value: 'bug', label: 'feedbackKindBug' },
  { value: 'idea', label: 'feedbackKindIdea' },
];

const PRIORITIES: Array<{ value: FeedbackPriority; label: StringKey }> = [
  { value: 'low', label: 'feedbackPriorityLow' },
  { value: 'medium', label: 'feedbackPriorityMedium' },
  { value: 'high', label: 'feedbackPriorityHigh' },
];

const router = useRouter();

const kind = ref<FeedbackKind>('bug');
const priority = ref<FeedbackPriority>('medium');
const title = ref('');
const body = ref('');

const mine = ref<Feedback[]>([]);
const remaining = ref(0);
const loaded = ref(false);
const busy = ref(false);
const error = ref('');
const sent = ref(false);

const titleField = ref<InstanceType<typeof OaTextField> | null>(null);
const guard = ref<InstanceType<typeof OaTurnstile> | null>(null);

const full = computed(() => loaded.value && remaining.value <= 0);

async function refresh(): Promise<void> {
  try {
    const result = await listFeedback();
    mine.value = result.feedback;
    remaining.value = result.remaining;
  } catch (failure) {
    error.value = failure instanceof ApiError ? failure.message : t('failed');
  } finally {
    loaded.value = true;
  }
}

async function send(): Promise<void> {
  if (busy.value) return;
  const heading = title.value.trim();
  if (!heading) {
    error.value = t('feedbackNeedTitle');
    titleField.value?.focus();
    return;
  }
  if (!body.value.trim()) {
    error.value = t('feedbackNeedBody');
    return;
  }

  busy.value = true;
  error.value = '';
  try {
    await sendFeedback({
      kind: kind.value,
      priority: priority.value,
      title: heading,
      body: body.value.trim(),
      turnstile: guard.value?.token() ?? '',
    });
    title.value = '';
    body.value = '';
    priority.value = 'medium';
    sent.value = true;
    // It stays until the next thing is typed, rather than for a couple of
    // seconds: somebody who has just pressed Send and looked away should
    // still find the answer when they look back.
    await refresh();
  } catch (failure) {
    error.value = failure instanceof ApiError ? failure.message : t('failed');
  } finally {
    busy.value = false;
    // A token is good for one submission, whether or not that submission was
    // accepted — so the next attempt needs a fresh one either way.
    guard.value?.reset();
  }
}

function touched(): void {
  sent.value = false;
}

function kindLabel(record: Feedback): string {
  return t(record.kind === 'bug' ? 'feedbackKindBug' : 'feedbackKindIdea');
}

function priorityLabel(value: FeedbackPriority): string {
  return t(PRIORITIES.find((entry) => entry.value === value)!.label);
}

onMounted(() => void refresh());
</script>

<template>
  <OaPanel
    :title="t('feedback')"
    :confirm-label="t('feedbackSend')"
    :confirmable="!full"
    :width="480"
    :busy="busy"
    :error="error"
    @close="router.push('/')"
    @confirm="send"
  >
    <p class="oa-field-hint">{{ t('feedbackIntro') }}</p>

    <div class="oa-field">
      <span class="oa-field-label">{{ t('feedbackKind') }}</span>
      <div class="oa-segmented">
        <button
          v-for="entry in KINDS"
          :key="entry.value"
          type="button"
          class="oa-segmented-option"
          :class="{ active: kind === entry.value }"
          @click="kind = entry.value; touched()"
        >{{ t(entry.label) }}</button>
      </div>
    </div>

    <div class="oa-field">
      <span class="oa-field-label">{{ t('feedbackPriority') }}</span>
      <div class="oa-segmented">
        <button
          v-for="entry in PRIORITIES"
          :key="entry.value"
          type="button"
          class="oa-segmented-option"
          :class="{ active: priority === entry.value }"
          @click="priority = entry.value; touched()"
        >{{ t(entry.label) }}</button>
      </div>
    </div>

    <OaTextField
      ref="titleField"
      v-model="title"
      :label="t('feedbackTitleLabel')"
      :placeholder="t('feedbackTitlePlaceholder')"
      :max-length="120"
      @update:model-value="touched"
    />

    <!-- Deliberately tall. A three-line box tells somebody to be brief, and
         the thing that makes a report worth having is the part they would
         have left out. -->
    <OaTextArea
      v-model="body"
      class="oa-feedback-body"
      :label="t('feedbackBodyLabel')"
      :placeholder="t('feedbackBodyPlaceholder')"
      :hint="t('feedbackBodyHint')"
      :rows="12"
      @update:model-value="touched"
    />

    <!-- Where the operator asked for one. The daily cap already holds one
         account to ten; this is what keeps a script holding somebody's cookie
         from spending those ten without a person present. -->
    <OaTurnstile
      v-if="siteInfo.turnstile_on_feedback"
      ref="guard"
      :site-key="siteInfo.turnstile_site_key ?? ''"
    />

    <p v-if="sent" class="oa-feedback-sent">{{ t('feedbackSent') }}</p>
    <p v-else-if="full" class="oa-feedback-full">{{ t('feedbackNoneLeft') }}</p>
    <p v-else-if="loaded" class="oa-field-hint">{{ t('feedbackRemaining', { count: remaining }) }}</p>

    <section class="oa-feedback-mine">
      <h3 class="oa-panel-section-title">{{ t('feedbackMine') }}</h3>
      <p v-if="!loaded" class="oa-menu-empty">{{ t('loading') }}</p>
      <p v-else-if="!mine.length" class="oa-menu-empty">{{ t('feedbackMineEmpty') }}</p>
      <ul v-else class="oa-feedback-list">
        <li v-for="record in mine" :key="record.id" class="oa-feedback-item">
          <div class="oa-feedback-item-head">
            <span class="oa-feedback-item-title">{{ record.title }}</span>
            <OaBadge :tone="record.status === 'resolved' ? 'muted' : 'default'">
              {{ t(record.status === 'resolved' ? 'feedbackStatusResolved' : 'feedbackStatusOpen') }}
            </OaBadge>
          </div>
          <div class="oa-feedback-item-meta">
            <span>{{ kindLabel(record) }}</span>
            <span>{{ priorityLabel(record.priority) }}</span>
            <span>{{ relativeTime(record.created_at) }}</span>
          </div>
        </li>
      </ul>
    </section>
  </OaPanel>
</template>
