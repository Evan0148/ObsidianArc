<script setup lang="ts">
// Invites: what an account can do with a code, in both directions.
//
// The claim box at the top is not gated on anything — a code somebody hands
// you is a code you might want to redeem whether or not this account's own
// personal invites are on, so it stays even when the card below it does not.
// The personal-code card is drawn in the shape SecuritySection's two-step
// panel already established, because both are "a code this account holds
// and can hand to somebody else". Its reward changed from one fixed payout
// per invite to a running count against invites.reward_every, so what used
// to be a sentence under the title is now a progress line and a bar, and
// each invitee row carries its own outcome rather than a shared blurb.

import { computed, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { ApiError } from '@/api/client';
import {
  claimInviteCode, fetchProfileInvites, formatInviteCode, inviteLink, regenerateProfileInvite,
  type ProfileInvitee, type ProfileInvites,
} from '@/api/invites';
import OaConfirmButton from '@/components/OaConfirmButton.vue';
import { t, tn } from '@/composables/useI18n';
import { IconCheck, IconCopy, IconKey, IconUsers } from '@/icons';
import { copyToClipboard } from '@/chat/markdown';
import { absoluteTime } from '@/lib/format';
import { matchesSettings } from './search';

const props = withDefaults(defineProps<{ query?: string }>(), { query: '' });
const route = useRoute();

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

// --- claiming a code (a partner's, or one minted to be claimable) ---------

const claimCode = ref('');
const claiming = ref(false);
const claimFlash = ref('');
const claimOk = ref(false);

onMounted(() => {
  // A link opened while already signed in is redirected here with the code
  // it carried (see the router guard) — worth pre-filling, since going to
  // find it again is the alternative. Never submitted on its own: claiming
  // changes the account's group, and that is a decision to make on purpose.
  const carried = route.query['claim'];
  if (typeof carried === 'string' && carried) claimCode.value = carried;
});

function claimErrorText(failure: unknown): string {
  if (!(failure instanceof ApiError)) return String(failure);
  switch (failure.code) {
    case 'invite_claimed': return t('inviteClaimAlready');
    case 'invite_group_conflict': return t('inviteClaimConflict');
    case 'too_many_attempts':
      return t('tooManyAttempts', { count: Number(failure.details['retry_after_seconds'] ?? 60) });
    case 'invite_invalid':
      return t('inviteClaimInvalid');
    default:
      return failure.message;
  }
}

async function submitClaim(): Promise<void> {
  const code = claimCode.value.trim();
  if (!code || claiming.value) return;
  claiming.value = true;
  claimFlash.value = '';
  try {
    const result = await claimInviteCode(code);
    claimOk.value = true;
    claimFlash.value = t('inviteClaimSuccess', { group: result.group_name, date: absoluteTime(result.expires_at) });
    claimCode.value = '';
    // The claimed group can be this account's own invite-reward group too,
    // so its card above may now read differently.
    void load();
  } catch (failure) {
    claimOk.value = false;
    claimFlash.value = claimErrorText(failure);
  } finally {
    claiming.value = false;
  }
}

// --- the every-N reward on this account's own personal code ---------------

/** 0–100: how far into the current cycle the account's counted total sits. */
const progressPercent = computed(() => {
  if (!data.value || data.value.reward_every <= 0) return 0;
  const filled = (data.value.reward_every - data.value.next_reward_in) / data.value.reward_every;
  return Math.max(0, Math.min(100, Math.round(filled * 100)));
});

/** One line per invitee: counted (with any cards that row itself earned),
 *  why it was skipped, or still pending. */
function statusLine(invitee: ProfileInvitee): string {
  const status = invitee.counted ? t('inviteeCounted') : skipLabel(invitee.reward_skipped);
  const cards = invitee.reward_cards > 0
    ? tn(invitee.reward_cards, 'inviteeCardsOne', 'inviteeCardsOther', { cards: invitee.reward_cards })
    : '';
  return cards ? `${status} · ${cards}` : status;
}

function skipLabel(reason: string): string {
  switch (reason) {
    case 'same_ip': return t('inviteeSkipSameIp');
    case 'limit': return t('inviteeSkipLimit');
    case 'disabled': return t('inviteeSkipDisabled');
    case 'inviter_gone': return t('inviteeSkipInviterGone');
    case 'inviter_disabled': return t('inviteeSkipInviterDisabled');
    default: return t('inviteePending');
  }
}
</script>

<template>
  <div v-if="data" v-show="matchesSettings(props.query, 'invites')" class="oa-settings-panel">
    <h2 class="oa-admin-section-title">{{ t('secInvites') }}</h2>

    <!-- Always present: a code from anyone else is worth redeeming whether
         or not this account has personal invites of its own. -->
    <section class="oa-2fa-panel">
      <header class="oa-2fa-head">
        <span class="oa-2fa-head-mark"><IconKey :size="18" /></span>
        <div class="oa-2fa-head-text">
          <div class="oa-2fa-head-title"><span>{{ t('inviteClaimTitle') }}</span></div>
          <p class="oa-2fa-head-meta">{{ t('inviteClaimHint') }}</p>
        </div>
      </header>

      <form class="oa-2fa-confirm" novalidate @submit.prevent="submitClaim">
        <div class="oa-2fa-confirm-row oa-field">
          <input
            v-model="claimCode"
            type="text"
            spellcheck="false"
            autocomplete="off"
            maxlength="32"
            :placeholder="t('inviteCodePlaceholder')"
            :aria-label="t('inviteClaimTitle')"
          >
          <button type="submit" class="oa-btn primary" :disabled="claiming || !claimCode.trim()">
            {{ claiming ? t('inviteClaiming') : t('inviteClaimSubmit') }}
          </button>
        </div>
      </form>
      <p v-if="claimFlash" class="oa-2fa-flash" :class="{ ok: claimOk }" role="alert">{{ claimFlash }}</p>
    </section>

    <section v-if="data.enabled" class="oa-2fa-panel">
      <header class="oa-2fa-head">
        <span class="oa-2fa-head-mark"><IconUsers :size="18" /></span>
        <div class="oa-2fa-head-text">
          <div class="oa-2fa-head-title"><span>{{ t('secInvites') }}</span></div>
          <p class="oa-2fa-head-meta">
            {{ data.limit > 0 ? t('inviteUsageLimited', { used: data.used, limit: data.limit }) : t('inviteUsageUnlimited', { used: data.used }) }}
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

      <!-- Hidden rather than shown at zero: a reward the operator switched
           off is not a milestone this account is failing to reach. -->
      <div v-if="data.reward_cards > 0" class="oa-2fa-panel-body">
        <p class="oa-2fa-note">
          {{ tn(data.reward_cards, 'inviteProgressOne', 'inviteProgressOther', {
            counted: data.counted, remaining: data.next_reward_in, cards: data.reward_cards,
          }) }}
        </p>
        <div class="oa-meter"><div class="oa-meter-fill" :style="{ width: `${progressPercent}%` }" /></div>
      </div>

      <template v-if="data.invitees.length">
        <div v-for="invitee in data.invitees" :key="invitee.username" class="oa-2fa-row">
          <div class="oa-2fa-row-text">
            <span class="oa-2fa-row-title">{{ invitee.nickname || invitee.username }}</span>
            <span class="oa-2fa-row-meta">{{ absoluteTime(invitee.created_at) }}</span>
          </div>
          <span class="oa-2fa-row-meta">{{ statusLine(invitee) }}</span>
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
