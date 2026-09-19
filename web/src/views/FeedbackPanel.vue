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
import OaIconButton from '@/components/OaIconButton.vue';
import OaOverlay from '@/components/OaOverlay.vue';
import OaPanel from '@/components/OaPanel.vue';
import OaTextArea from '@/components/OaTextArea.vue';
import OaTextField from '@/components/OaTextField.vue';
import OaTurnstile from '@/components/OaTurnstile.vue';
import { t, type StringKey } from '@/composables/useI18n';
import { IconClose, IconLock } from '@/icons';
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

/**
 * The challenge, when there is one, is a sheet rather than a row in the form.
 *
 * Cloudflare's widget is a fixed 300px box with its own chrome, and a panel
 * this narrow — resizable down to 320px — squeezed it against the sides. The
 * redemption dialog had already answered this: the check gets a surface of
 * its own, and solving it sends what the reader had already written.
 */
const challengeOpen = ref(false);
const challengeError = ref('');

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

function send(): void {
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

  error.value = '';
  if (siteInfo.value.turnstile_on_feedback) {
    challengeError.value = '';
    challengeOpen.value = true;
    return;
  }
  void submit('');
}

async function submit(token: string): Promise<void> {
  busy.value = true;
  try {
    await sendFeedback({
      kind: kind.value,
      priority: priority.value,
      title: title.value.trim(),
      body: body.value.trim(),
      ...(token ? { turnstile: token } : {}),
    });
    title.value = '';
    body.value = '';
    priority.value = 'medium';
    challengeOpen.value = false;
    sent.value = true;
    // It stays until the next thing is typed, rather than for a couple of
    // seconds: somebody who has just pressed Send and looked away should
    // still find the answer when they look back.
    await refresh();
  } catch (failure) {
    const code = failure instanceof ApiError ? failure.code : '';
    // A refused check is answered inside the sheet, with the widget reset —
    // a token is spent whether or not it passed — so the reader can try again
    // without losing what they wrote.
    if (token && (code === 'challenge_failed' || code === 'challenge_unavailable')) {
      challengeError.value = code === 'challenge_failed'
        ? t('challengeFailed') : t('challengeUnavailable');
      guard.value?.reset();
    } else {
      challengeOpen.value = false;
      error.value = failure instanceof ApiError ? failure.message : t('failed');
    }
  } finally {
    busy.value = false;
  }
}

async function solved(): Promise<void> {
  const token = guard.value?.token() ?? '';
  if (!token || busy.value) return;
  challengeError.value = '';
  await submit(token);
}

function cancelChallenge(): void {
  challengeOpen.value = false;
  challengeError.value = '';
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

  <!-- Where the operator asked for one. The daily cap already holds one
       account to ten a day; this is what keeps a script holding somebody's
       cookie from spending those ten without a person present. -->
  <OaOverlay
    v-if="challengeOpen"
    v-slot="{ close }"
    overlay-class="oa-modal-overlay"
    :dismissible="!busy"
    @close="cancelChallenge"
  >
    <div class="oa-auth-card oa-modal-card">
      <OaIconButton class="oa-icon-btn oa-modal-close" :label="t('close')" @click="close">
        <IconClose :size="16" />
      </OaIconButton>

      <div class="oa-auth-brand">
        <span class="oa-auth-mark"><IconLock :size="15" /></span>
        <span>{{ siteInfo.name }}</span>
      </div>
      <h1 class="oa-auth-title">{{ t('feedbackChallengeTitle') }}</h1>
      <p class="oa-auth-sub">{{ t('feedbackChallengeBody') }}</p>

      <div class="oa-auth-form">
        <OaTurnstile
          ref="guard"
          :site-key="siteInfo.turnstile_site_key ?? ''"
          @solved="solved"
        />
        <p v-if="challengeError" class="oa-auth-error" role="alert">{{ challengeError }}</p>
      </div>
    </div>
  </OaOverlay>
</template>
