<script setup lang="ts">
// An account's own invite code: the personal-referral half of the invite
// system (the admin-issued half — batch codes, partner links — lives in the
// backoffice). Drawn as one card in the shape SecuritySection's two-step
// panel already established, because both are "a code this account holds and
// can hand to somebody else".
//
// Hidden outright rather than shown empty when the operator has invites off:
// an account with nothing to copy and nobody to see get here has no reason to
// see a card explaining that.

import { computed, onMounted, ref } from 'vue';
import { ApiError } from '@/api/client';
import {
  fetchProfileInvites, formatInviteCode, inviteLink, regenerateProfileInvite, type ProfileInvites,
} from '@/api/invites';
import OaConfirmButton from '@/components/OaConfirmButton.vue';
import { t, tn } from '@/composables/useI18n';
import { IconCheck, IconCopy, IconUsers } from '@/icons';
import { copyToClipboard } from '@/chat/markdown';
import { absoluteTime } from '@/lib/format';
import { matchesSettings } from './search';

const props = withDefaults(defineProps<{ query?: string }>(), { query: '' });

const data = ref<ProfileInvites | null>(null);
const flash = ref('');
const busy = ref(false);
const copiedCode = ref(false);
const copiedLink = ref(false);

const formattedCode = computed(() => (data.value ? formatInviteCode(data.value.code) : ''));
const link = computed(() => (data.value ? inviteLink(data.value.code) : ''));

async function load(): Promise<void> {
  try {
    data.value = await fetchProfileInvites();
  } catch (failure) {
    // The card only earns a place on screen once `data.enabled` is known
    // true, so a failure here before that leaves nothing to show it on —
    // `flash` still exists for a regenerate that fails after loading did not.
    flash.value = failure instanceof ApiError ? failure.message : String(failure);
  }
}

onMounted(load);

function flashCopy(which: 'code' | 'link'): void {
  const target = which === 'code' ? copiedCode : copiedLink;
  target.value = true;
  window.setTimeout(() => { target.value = false; }, 1500);
}

// A refused clipboard — plain HTTP, a locked-down browser — says so, the
// way the backoffice's copy buttons do, so the reader knows to select the
// text by hand instead of pasting whatever was there before.
function copied(which: 'code' | 'link', ok: boolean): void {
  if (ok) flashCopy(which);
  else flash.value = t('copyFailed');
}

function copyCode(): void {
  if (!data.value) return;
  void copyToClipboard(data.value.code).then((ok) => copied('code', ok));
}

function copyLink(): void {
  void copyToClipboard(link.value).then((ok) => copied('link', ok));
}

async function regenerate(): Promise<void> {
  if (busy.value) return;
  busy.value = true;
  flash.value = '';
  try {
    data.value = await regenerateProfileInvite();
  } catch (failure) {
    flash.value = failure instanceof ApiError ? failure.message : String(failure);
  } finally {
    busy.value = false;
  }
}

/** One line per invitee: joined, rewarded, or why a reward did not land. */
function statusLabel(invitee: ProfileInvites['invitees'][number]): string {
  if (invitee.rewarded) return t('inviteeRewarded');
  switch (invitee.reward_skipped) {
    case 'same_ip': return t('inviteeSkipSameIp');
    case 'limit': return t('inviteeSkipLimit');
    case 'disabled': return t('inviteeSkipDisabled');
    default: return t('inviteePending');
  }
}
</script>

<template>
  <div v-if="data && data.enabled" v-show="matchesSettings(props.query, 'invites')" class="oa-settings-panel">
    <h2 class="oa-admin-section-title">{{ t('secInvites') }}</h2>

    <section class="oa-2fa-panel">
      <header class="oa-2fa-head">
        <span class="oa-2fa-head-mark"><IconUsers :size="18" /></span>
        <div class="oa-2fa-head-text">
          <div class="oa-2fa-head-title"><span>{{ t('secInvites') }}</span></div>
          <p class="oa-2fa-head-meta">
            {{ data.reward_cards > 0
              ? tn(data.reward_cards, 'inviteRewardOne', 'inviteRewardOther', { cards: data.reward_cards, days: data.reward_card_days })
              : t('inviteRewardNone') }}
          </p>
        </div>
      </header>

      <div class="oa-2fa-key">
        <div class="oa-2fa-key-row">
          <span>{{ t('inviteYourCode') }}</span>
          <button type="button" class="oa-2fa-secret" @click="copyCode">
            <IconCheck v-if="copiedCode" :size="14" />
            <IconCopy v-else :size="14" />
            {{ formattedCode }}
          </button>
        </div>
        <div class="oa-2fa-key-row">
          <span>{{ t('inviteYourLink') }}</span>
          <button type="button" class="oa-2fa-secret" @click="copyLink">
            <IconCheck v-if="copiedLink" :size="14" />
            <IconCopy v-else :size="14" />
            {{ copiedLink ? t('copied') : t('copy') }}
          </button>
        </div>
      </div>

      <p class="oa-2fa-note">
        {{ data.limit > 0 ? t('inviteUsageLimited', { used: data.used, limit: data.limit }) : t('inviteUsageUnlimited', { used: data.used }) }}
      </p>

      <template v-if="data.invitees.length">
        <div v-for="invitee in data.invitees" :key="invitee.username" class="oa-2fa-row">
          <div class="oa-2fa-row-text">
            <span class="oa-2fa-row-title">{{ invitee.nickname || invitee.username }}</span>
            <span class="oa-2fa-row-meta">{{ absoluteTime(invitee.created_at) }}</span>
          </div>
          <span class="oa-2fa-row-meta">{{ statusLabel(invitee) }}</span>
        </div>
      </template>
      <p v-else class="oa-2fa-note">{{ t('inviteesEmpty') }}</p>

      <div class="oa-2fa-row">
        <div class="oa-2fa-row-text">
          <span class="oa-2fa-row-title">{{ t('inviteRegenerate') }}</span>
          <span class="oa-2fa-row-meta">{{ t('inviteRegenerateHint') }}</span>
        </div>
        <OaConfirmButton
          class="oa-btn"
          :label="t('inviteRegenerate')"
          :armed-label="t('confirmWord')"
          :armed-title="t('inviteRegenerateConfirm')"
          :resting-title="t('inviteRegenerate')"
          :disabled="busy"
          @confirm="regenerate"
        />
      </div>

      <p v-if="flash" class="oa-2fa-flash" role="alert">{{ flash }}</p>
    </section>
  </div>
</template>
