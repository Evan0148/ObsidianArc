<script setup lang="ts">
// The chat surface: the rail beside the transcript, the composer under it.
//
// Both are columns of the row this is rendered into, which is also where a
// side panel arrives — so /settings and /keys narrow the conversation rather
// than covering it.

import { nextTick, ref, watch } from 'vue';
import { useResizeObserver } from '@vueuse/core';
import OaIconButton from '@/components/OaIconButton.vue';
import OaScrollArea from '@/components/OaScrollArea.vue';
import { t } from '@/composables/useI18n';
import { isWork, pendingMode, setMode, type Mode } from '@/stores/workspace';
import { IconMenu, IconPlus } from '@/icons';
import ChatComposer from './ChatComposer.vue';
import ChatChallenge from './ChatChallenge.vue';
import ChatMessage from './ChatMessage.vue';
import ChatPending from './ChatPending.vue';
import ChatSidebar from './ChatSidebar.vue';
import {
  active, addImages, busy, chatChallenge, dragging, draft, flash, historyOpen, messages, pending,
  scrollTick, showPending, startNewConversation, status, submit, suggestions, switchTick,
} from './useChat';
import { isAdmin } from '@/stores/session';

// The two surfaces, in the order they are offered. A list rather than two
// hand-written buttons so the strip cannot drift out of step with the store
// that holds which one is chosen.
const MODES: Mode[] = ['chat', 'work'];

const emit = defineEmits<{ (event: 'open-setup'): void }>();

const scroll = ref<InstanceType<typeof OaScrollArea> | null>(null);
const composer = ref<InstanceType<typeof ChatComposer> | null>(null);
const transcriptRef = ref<HTMLElement | null>(null);
const autoScroll = ref(true);
const rising = ref(false);

function scroller(): HTMLElement | null {
  return scroll.value?.scroller ?? null;
}

function scrollToBottom(): void {
  const node = scroller();
  if (!node) return;
  node.scrollTop = node.scrollHeight;
}

function onScroll(): void {
  const node = scroller();
  if (!node) return;
  autoScroll.value = node.scrollHeight - node.scrollTop - node.clientHeight < 60;
}

// Auto-follow when content grows (streaming deltas, MathML equations upgraded, images loaded).
useResizeObserver(transcriptRef, () => {
  if (autoScroll.value) {
    scrollToBottom();
  }
});

/**
 * Auto-scroll when the reader is following at the bottom or when
 * completing a turn: yanking the view back while somebody is reading an earlier
 * part of the answer is avoided.
 */
watch(scrollTick, () => {
  if (autoScroll.value || !busy.value) {
    void nextTick(scrollToBottom);
  }
});

// A short rise says "a different conversation" instead of leaving the
// transcript to flicker into something else within one frame.
watch(switchTick, () => {
  autoScroll.value = true;
  rising.value = false;
  void nextTick(() => {
    scrollToBottom();
    rising.value = true;
    window.setTimeout(() => { rising.value = false; }, 400);
  });
});

function carriesFiles(event: DragEvent): boolean {
  if (!status.value.vision) return false;
  const types = event.dataTransfer?.types;
  return !!types && Array.prototype.indexOf.call(types, 'Files') !== -1;
}

function onDragOver(event: DragEvent): void {
  if (!carriesFiles(event)) return;
  event.preventDefault();
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy';
  dragging.value = true;
}

function onDragLeave(event: DragEvent): void {
  const main = event.currentTarget as HTMLElement;
  if (event.target === main || !main.contains(event.relatedTarget as Node | null)) {
    dragging.value = false;
  }
}

function onDrop(event: DragEvent): void {
  if (!carriesFiles(event)) return;
  event.preventDefault();
  dragging.value = false;
  void addImages(event.dataTransfer?.files ?? null);
}

function ask(key: (typeof suggestions.value)[number]): void {
  draft.value = t(key);
  void submit();
}

defineExpose({ focus: () => composer.value?.focus() });
</script>

<template>
  <ChatSidebar />

  <div class="ai-chat-main" @dragenter="onDragOver" @dragover="onDragOver" @dragleave="onDragLeave" @drop="onDrop">
    <!-- The wide skin hides this bar in favour of the workspace header; it is
         the navigation on a narrow screen, where the rail is an overlay. -->
    <div class="ai-chat-bar">
      <OaIconButton
        class="ai-chat-bar-btn"
        :label="t('history')"
        @click="historyOpen = !historyOpen"
      ><IconMenu :size="16" /></OaIconButton>
      <span class="ai-chat-bar-title">{{ active?.title || t('brand') }}</span>
      <OaIconButton class="ai-chat-bar-btn" :label="t('newChat')" @click="startNewConversation">
        <IconPlus :size="15" />
      </OaIconButton>
    </div>

    <OaScrollArea
      ref="scroll"
      wrap-class="ai-chat-scroll-wrap"
      :scroll-class="rising ? 'ai-chat-scroll ai-chat-switching' : 'ai-chat-scroll'"
      @scroll.passive="onScroll"
    >
      <div v-if="!status.configured" class="ai-chat-setup">
        <h3 class="ai-chat-setup-title">{{ t('setupTitle') }}</h3>
        <p class="ai-chat-setup-body">{{ isAdmin ? t('setupBody') : t('setupBodyUser') }}</p>
        <button
          v-if="isAdmin"
          type="button"
          class="ai-chat-setup-action"
          @click="emit('open-setup')"
        >{{ t('setupAction') }}</button>
      </div>

      <div ref="transcriptRef" class="ai-chat-transcript">
        <ChatMessage v-for="message in messages" :key="message.id" :message="message" />
        <ChatPending v-if="showPending && pending" :pending="pending" />
      </div>

      <div v-if="!messages.length && status.configured" class="ai-chat-empty">
        <!-- Only on the empty state, because that is the only moment the
             choice is still open: once a conversation exists it carries its
             own mode, and a toggle over a running thread would offer to
             change something it cannot. -->
        <div class="ai-mode-switch" role="tablist" :aria-label="t('modeChat')">
          <button
            v-for="option in MODES"
            :key="option"
            type="button"
            role="tab"
            class="ai-mode-choice"
            :class="{ active: pendingMode === option }"
            :aria-selected="pendingMode === option"
            @click="setMode(option)"
          >{{ t(option === 'work' ? 'modeWork' : 'modeChat') }}</button>
        </div>

        <h3 class="ai-chat-empty-title">{{ isWork ? t('workGreeting') : t('emptyTitle') }}</h3>
        <p class="ai-chat-empty-body">{{ isWork ? t('workBlurb') : t('emptyBody') }}</p>
        <div v-if="!isWork" class="ai-chat-suggestions">
          <button
            v-for="key in suggestions"
            :key="key"
            type="button"
            class="ai-chat-suggestion"
            @click="ask(key)"
          >{{ t(key) }}</button>
        </div>
      </div>
    </OaScrollArea>

    <div class="ai-chat-flash" :class="{ visible: !!flash }" role="status" aria-live="polite">
      {{ flash }}
    </div>

    <ChatComposer ref="composer" />

    <div class="ai-chat-drop">{{ t('dropHint') }}</div>
  </div>

  <ChatChallenge v-if="chatChallenge" />
</template>
