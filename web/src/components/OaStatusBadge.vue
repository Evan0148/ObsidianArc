<script setup lang="ts">
// How one request ended, in a colour.
//
// A reusable badge row for turn and request ledgers across the application:
// the user's usage panel, the operator's ledger and the dashboard recent-request
// table all share this shape.

import OaBadge from './OaBadge.vue';
import OaBadgeRow from './OaBadgeRow.vue';
import { t } from '@/composables/useI18n';
import { formatErrorCode, errorCodeDescription } from '@/lib/errors';

const props = defineProps<{
  status: string;
  errorCode?: string | undefined;
  /** When true, omits the badge completely for 'ok' rows (useful in user turns list). */
  hideOk?: boolean;
}>();
</script>

<template>
  <template v-if="props.status === 'ok'">
    <OaBadge v-if="!props.hideOk" tone="muted">{{ t('statusOk') }}</OaBadge>
  </template>
  <OaBadge v-else-if="props.status === 'aborted'" tone="muted">{{ t('statusStopped') }}</OaBadge>
  <OaBadgeRow v-else :title="errorCodeDescription(props.errorCode)">
    <OaBadge tone="danger">
      {{ props.status === 'rejected' ? t('statusRefused') : t('statusFailed') }}
    </OaBadge>
    <OaBadge v-if="props.errorCode" tone="muted">{{ formatErrorCode(props.errorCode) }}</OaBadge>
  </OaBadgeRow>
</template>
