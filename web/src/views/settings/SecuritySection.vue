<script setup lang="ts">
// The account's second sign-in step: whether it is on, the way to turn it on,
// and the two things done to it afterwards — new recovery codes, and turning
// it off.
//
// Both of those take a code, because a session somebody walked away from is
// exactly what they must not be enough for. The field for it opens under the
// row that asked for it, so the resting state is one card with a status line
// and two rows rather than a form nobody is filling in. The setup wizard opens
// inside the same card, so the thing being set up and the steps setting it up
// read as one object instead of a stack of loose parts.

import { computed, onMounted, ref } from 'vue';
import type { Account } from '@/api/auth';
import { ApiError } from '@/api/client';
import {
  disableTwoFactor, fetchTwoFactor, regenerateRecovery, type TwoFactorStatus,
} from '@/api/twofactor';
import { copyToClipboard } from '@/chat/markdown';
import OaBadge from '@/components/OaBadge.vue';
import { t } from '@/composables/useI18n';
import { IconCheck, IconCopy, IconShield } from '@/icons';
import { absoluteTime } from '@/lib/format';
import { adopt, currentPreferences } from '@/stores/session';
import { matchesSettings } from './search';
import TwoFactorWizard from './TwoFactorWizard.vue';

const props = withDefaults(defineProps<{ query?: string }>(), { query: '' });

const status = ref<TwoFactorStatus | null>(null);
const flash = ref('');
const notice = ref('');
const wizard = ref(false);

/** Which code-taking action is open, if any. */
const action = ref<'' | 'disable' | 'recovery'>('');
const code = ref('');
const busy = ref(false);
const fresh = ref<string[]>([]);
const copied = ref(false);

/** Few enough left that running out is a real prospect. */
const running = computed(() => !!status.value?.enabled && status.value.recovery_remaining <= 3);

async function load(): Promise<void> {
  try {
    status.value = await fetchTwoFactor();
  } catch (failure) {
    flash.value = failure instanceof ApiError ? failure.message : String(failure);
  }
}

onMounted(load);

function enrolled(user: Account): void {
  adopt(user, currentPreferences.value);
  wizard.value = false;
  notice.value = t('twoFactorEnabledNotice');
  void load();
}

function open(next: 'disable' | 'recovery'): void {
  action.value = action.value === next ? '' : next;
  code.value = '';
  flash.value = '';
  fresh.value = [];
}

function refusal(failure: unknown): string {
  if (!(failure instanceof ApiError)) return String(failure);
  switch (failure.code) {
    case 'two_factor_code': return t('twoFactorCodeWrong');
    case 'two_factor_mandatory': return t('twoFactorMandatoryNote');
    default: return failure.message;
  }
}

async function confirm(): Promise<void> {
  if (busy.value || !code.value.trim()) return;
  busy.value = true;
  flash.value = '';
  notice.value = '';
  try {
    if (action.value === 'disable') {
      const { user } = await disableTwoFactor(code.value.trim());
      adopt(user, currentPreferences.value);
      notice.value = t('twoFactorTurnedOff');
    } else {
      const { recovery_codes: codes } = await regenerateRecovery(code.value.trim());
      fresh.value = codes;
    }
    action.value = action.value === 'recovery' ? 'recovery' : '';
    code.value = '';
    await load();
  } catch (failure) {
    flash.value = refusal(failure);
  } finally {
    busy.value = false;
  }
}

function copyFresh(): void {
  void copyToClipboard(fresh.value.join('\n')).then((ok) => {
    if (!ok) return;
    copied.value = true;
    window.setTimeout(() => { copied.value = false; }, 1500);
  });
}
</script>

<template>
  <div v-show="matchesSettings(props.query, 'twofactor')" class="oa-settings-panel">
    <h2 class="oa-admin-section-title">{{ t('secTwoFactor') }}</h2>

    <section v-if="status" class="oa-2fa-panel">
      <header class="oa-2fa-head">
        <span class="oa-2fa-head-mark"><IconShield :size="18" /></span>
        <div class="oa-2fa-head-text">
          <div class="oa-2fa-head-title">
            <span>{{ t('twoFactorAuthenticator') }}</span>
            <OaBadge :tone="status.enabled ? 'default' : 'muted'">
              {{ status.enabled ? t('twoFactorOn') : t('twoFactorOff') }}
            </OaBadge>
          </div>
          <p class="oa-2fa-head-meta">
            {{ status.enabled ? t('twoFactorOnSince', { date: absoluteTime(status.enabled_at) }) : t('twoFactorIntro') }}
          </p>
        </div>
        <button
          v-if="!status.enabled && status.available && !wizard"
          type="button"
          class="oa-btn primary"
          @click="wizard = true; notice = ''"
        >
          {{ t('twoFactorTurnOn') }}
        </button>
      </header>

      <p v-if="!status.available" class="oa-2fa-note">{{ t('twoFactorUnavailable') }}</p>
      <p v-if="status.mandatory" class="oa-2fa-note">
        {{ status.enabled ? t('twoFactorMandatoryNote') : t('twoFactorMandatoryOff') }}
      </p>

      <div v-if="wizard && !status.enabled" class="oa-2fa-panel-body">
        <TwoFactorWizard cancellable @done="enrolled" @cancel="wizard = false" />
      </div>

      <template v-if="status.enabled">
        <div class="oa-2fa-row">
          <div class="oa-2fa-row-text">
            <span class="oa-2fa-row-title">{{ t('twoFactorRecoveryRow') }}</span>
            <span class="oa-2fa-row-meta" :class="{ warn: running }">
              {{ running
                ? t('twoFactorRecoveryLow', { count: status.recovery_remaining })
                : t('twoFactorRecoveryRowHint', { count: status.recovery_remaining }) }}
            </span>
          </div>
          <button type="button" class="oa-btn" :class="{ primary: action === 'recovery' }" @click="open('recovery')">
            {{ t('twoFactorRegenerateShort') }}
          </button>
        </div>
        <form v-if="action === 'recovery' && !fresh.length" class="oa-2fa-confirm" novalidate @submit.prevent="confirm">
          <p class="oa-2fa-desc">{{ t('twoFactorRegenerateHint') }} {{ t('twoFactorConfirmWithCode') }}</p>
          <div class="oa-2fa-confirm-row oa-field">
            <input
              v-model="code"
              class="oa-2fa-code"
              type="text"
              autocomplete="one-time-code"
              spellcheck="false"
              maxlength="16"
              placeholder="000000"
              :aria-label="t('twoFactorCodeLabel')"
            >
            <button type="button" class="oa-btn" :disabled="busy" @click="action = ''">{{ t('cancel') }}</button>
            <button type="submit" class="oa-btn primary" :disabled="busy || !code.trim()">
              {{ t('twoFactorRegenerateShort') }}
            </button>
          </div>
        </form>
        <div v-if="fresh.length" class="oa-2fa-confirm">
          <p class="oa-2fa-desc">{{ t('twoFactorSaveBody') }}</p>
          <div class="oa-2fa-codes-box">
            <ol class="oa-2fa-codes">
              <li v-for="entry in fresh" :key="entry">{{ entry }}</li>
            </ol>
            <div class="oa-2fa-codes-actions">
              <button type="button" class="oa-btn" @click="copyFresh">
                <IconCheck v-if="copied" :size="14" />
                <IconCopy v-else :size="14" />
                {{ copied ? t('copied') : t('copy') }}
              </button>
              <button type="button" class="oa-btn primary" @click="fresh = []; action = ''">
                {{ t('twoFactorFinish') }}
              </button>
            </div>
          </div>
        </div>

        <div v-if="!status.mandatory" class="oa-2fa-row">
          <div class="oa-2fa-row-text">
            <span class="oa-2fa-row-title">{{ t('twoFactorTurnOff') }}</span>
            <span class="oa-2fa-row-meta">{{ t('twoFactorTurnOffHint') }}</span>
          </div>
          <button type="button" class="oa-btn" :class="{ primary: action === 'disable' }" @click="open('disable')">
            {{ t('twoFactorTurnOffShort') }}
          </button>
        </div>
        <form v-if="action === 'disable'" class="oa-2fa-confirm" novalidate @submit.prevent="confirm">
          <p class="oa-2fa-desc">{{ t('twoFactorConfirmWithCode') }}</p>
          <div class="oa-2fa-confirm-row oa-field">
            <input
              v-model="code"
              class="oa-2fa-code"
              type="text"
              autocomplete="one-time-code"
              spellcheck="false"
              maxlength="16"
              placeholder="000000"
              :aria-label="t('twoFactorCodeLabel')"
            >
            <button type="button" class="oa-btn" :disabled="busy" @click="action = ''">{{ t('cancel') }}</button>
            <button type="submit" class="oa-btn primary" :disabled="busy || !code.trim()">
              {{ t('twoFactorTurnOffShort') }}
            </button>
          </div>
        </form>
      </template>

      <p v-if="notice" class="oa-2fa-flash ok" role="status">{{ notice }}</p>
      <p v-if="flash" class="oa-2fa-flash" role="alert">{{ flash }}</p>
    </section>
    <p v-else-if="flash" class="oa-drawer-flash visible" role="alert">{{ flash }}</p>
  </div>
</template>
