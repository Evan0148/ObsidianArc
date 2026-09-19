<script setup lang="ts">
// One command the model ran during a work turn.
//
// Collapsed by default: a work session can call several of these in a row,
// and a wall of expanded arguments and output would bury the prose answer
// that follows them, which is the thing the reader actually came for.

import { computed } from 'vue';
import { t } from '@/composables/useI18n';
import type { ToolCallEntry } from '@/api/chat';

const props = defineProps<{ call: ToolCallEntry }>();

const failed = computed(() => props.call.done && props.call.failed === true);

const stateLabel = computed(() => {
  if (!props.call.done) return t('toolRunning');
  return failed.value ? t('toolFailed') : t('toolDone');
});
</script>

<template>
  <details class="ai-tool-call">
    <summary class="ai-tool-call-head">
      <span class="ai-tool-call-name">{{ props.call.name }}</span>
      <span class="ai-tool-call-tag" :class="{ warn: failed }">
        <span v-if="!props.call.done" class="ai-chat-spinner ai-tool-call-spinner" />
        {{ stateLabel }}
      </span>
    </summary>
    <div class="ai-tool-call-body">
      <div class="ai-tool-call-section">
        <div class="ai-tool-call-label">{{ t('toolArguments') }}</div>
        <pre class="ai-tool-call-pre">{{ props.call.arguments }}</pre>
      </div>
      <!-- Nothing to show yet while the call is still running; the section
           would just be an empty box under an empty label. -->
      <div v-if="props.call.done" class="ai-tool-call-section">
        <div class="ai-tool-call-label">{{ t('toolOutput') }}</div>
        <pre class="ai-tool-call-pre">{{ props.call.output }}</pre>
      </div>
    </div>
  </details>
</template>
