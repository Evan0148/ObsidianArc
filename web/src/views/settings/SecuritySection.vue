<script setup lang="ts">
// The account's second sign-in step: whether it is on, the way to turn it on,
// and the two things done to it afterwards — new recovery codes, and turning
// it off.
//
// Both of those take a code, because a session somebody walked away from is
// exactly what they must not be enough for. The field for it appears only
// once one of them has been chosen, so the resting state is a status line and
// two buttons rather than a form nobody is filling in.

import { computed, onMounted, ref } from 'vue';
import type { Account } from '@/api/auth';
import { ApiError } from '@/api/client';
import {
  disableTwoFactor, fetchTwoFactor, regenerateRecovery, type TwoFactorStatus,
} from '@/api/twofactor';
import { copyToClipboard } from '@/chat/markdown';
import OaBadge from '@/components/OaBadge.vue';
import OaField from '@/components/OaField.vue';
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
    <p class="oa-field-hint">{{ t('twoFactorIntro') }}</p>

    <div v-if="status" class="oa-connection oa-2fa-status">
      <span class="oa-connection-mark"><IconShield :size="16" /></span>
      <span class="oa-connection-body">
        <span class="oa-connection-name">
          {{ t('secTwoFactor') }}
          <OaBadge :tone="status.enabled ? 'default' : 'muted'">
            {{ status.enabled ? t('twoFactorOn') : t('twoFactorOff') }}
          </OaBadge>
        </span>
        <span class="oa-connection-meta">
          <template v-if="status.enabled">
            {{ t('twoFactorOnSince', { date: absoluteTime(status.enabled_at) }) }}
            · {{ t('twoFactorRecoveryLeft', { count: status.recovery_remaining }) }}
          </template>
          <template v-else>{{ t('twoFactorRecommended') }}</template>
        </span>
      </span>
    </div>

    <p v-if="status && !status.available" class="oa-field-hint">{{ t('twoFactorUnavailable') }}</p>
    <p v-if="status?.mandatory" class="oa-field-hint">{{ t('twoFactorMandatoryNote') }}</p>
    <p v-if="running" class="oa-auth-error" role="status">
      {{ t('twoFactorRecoveryLow', { count: status?.recovery_remaining ?? 0 }) }}
    </p>
    <p class="oa-drawer-flash ok" :class="{ visible: !!notice }" role="status">{{ notice }}</p>

    <template v-if="status && !status.enabled && status.available">
      <TwoFactorWizard v-if="wizard" cancellable @done="enrolled" @cancel="wizard = false" />
      <div v-else class="oa-button-row">
        <button type="button" class="oa-btn primary" @click="wizard = true; notice = ''">
          {{ t('twoFactorSetUp') }}
        </button>
      </div>
    </template>

    <template v-else-if="status?.enabled">
      <div class="oa-button-row">
        <button type="button" class="oa-btn" :class="{ primary: action === 'recovery' }" @click="open('recovery')">
          {{ t('twoFactorRegenerate') }}
        </button>
        <button
          v-if="!status.mandatory"
          type="button"
          class="oa-btn"
          :class="{ primary: action === 'disable' }"
          @click="open('disable')"
        >
          {{ t('twoFactorTurnOff') }}
        </button>
      </div>

      <form v-if="action && !fresh.length" class="oa-2fa-form" novalidate @submit.prevent="confirm">
        <p class="oa-field-hint">
          {{ action === 'disable' ? t('twoFactorTurnOffHint') : t('twoFactorRegenerateHint') }}
        </p>
        <OaField :label="t('twoFactorCodeLabel')" :hint="t('twoFactorConfirmWithCode')">
          <input
            v-model="code"
            class="oa-2fa-code"
            type="text"
            autocomplete="one-time-code"
            spellcheck="false"
            maxlength="16"
            placeholder="000000"
          >
        </OaField>
        <div class="oa-button-row">
          <button type="button" class="oa-btn" :disabled="busy" @click="action = ''">{{ t('cancel') }}</button>
          <button type="submit" class="oa-btn primary" :disabled="busy || !code.trim()">
            {{ action === 'disable' ? t('twoFactorTurnOff') : t('twoFactorRegenerate') }}
          </button>
        </div>
      </form>

      <template v-if="fresh.length">
        <p class="oa-field-hint">{{ t('twoFactorSaveBody') }}</p>
        <ul class="oa-2fa-codes mono">
          <li v-for="entry in fresh" :key="entry">{{ entry }}</li>
        </ul>
        <div class="oa-button-row">
          <button type="button" class="oa-btn" @click="copyFresh">
            <IconCheck v-if="copied" :size="14" />
            <IconCopy v-else :size="14" />
            {{ copied ? t('copied') : t('copy') }}
          </button>
          <button type="button" class="oa-btn primary" @click="fresh = []; action = ''">
            {{ t('twoFactorFinish') }}
          </button>
        </div>
      </template>
    </template>

    <p class="oa-drawer-flash" :class="{ visible: !!flash }" role="alert">{{ flash }}</p>
  </div>
</template>
