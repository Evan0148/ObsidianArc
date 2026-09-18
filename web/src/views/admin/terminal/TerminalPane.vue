<script setup lang="ts">
// One tab's scrollback and prompt line: CONTRACT.md #5.2.
//
// Only the active tab's pane is mounted (`AdminTerminal.vue` keys it behind
// a `v-if`) — a background tab's command keeps running because the fetch
// lives in its `TerminalSession`, not in this component, and switching back
// just renders whatever the session's refs already hold. That is also why
// this file owns no timer or `AbortController` of its own to release on
// teardown: everything it reads is either a prop or cleaned up by the
// `useResizeObserver` / vueuse composables below, which release themselves.

import { nextTick, onMounted, ref, watch } from 'vue';
import { useResizeObserver } from '@vueuse/core';
import type { ConsoleCompletionItem } from '@/api/console';
import OaIconButton from '@/components/OaIconButton.vue';
import OaScrollArea from '@/components/OaScrollArea.vue';
import { t } from '@/composables/useI18n';
import { IconStop } from '@/icons';
import type { TerminalSession } from './session';
import TerminalLine from './TerminalLine.vue';

const props = defineProps<{
  session: TerminalSession;
  fontSize: number;
  timestamps: boolean;
}>();

/** The measured column count, for `execConsoleLine`'s `cols` — CONTRACT.md #4 says it feeds `render.Table`'s own width maths server-side. */
const emit = defineEmits<{ (event: 'measure', cols: number): void }>();

const scrollComp = ref<InstanceType<typeof OaScrollArea> | null>(null);
const inputRef = ref<HTMLTextAreaElement | null>(null);
const completions = ref<ConsoleCompletionItem[]>([]);
/** False once the reader has scrolled up — CONTRACT.md #5.2's "auto-follow unless scrolled up". */
const following = ref(true);

const MAX_INPUT_HEIGHT = 160;
const FOLLOW_THRESHOLD = 24;
// A monospace glyph's advance width as a fraction of its font-size — close
// enough for `cols` to size the server's table output sensibly. A real
// measurement (a hidden probe span) would cost a layout on every keystroke
// that resizes the pane, for a number the server only uses to decide how
// wide to draw a table; this estimate is what that number is worth.
const CHAR_WIDTH_RATIO = 0.6015;

function estimateColumns(widthPx: number, fontSizePx: number): number {
  if (widthPx <= 0 || fontSizePx <= 0) return 80;
  return Math.max(20, Math.floor(widthPx / (fontSizePx * CHAR_WIDTH_RATIO)));
}

useResizeObserver(() => scrollComp.value?.scroller, (entries) => {
  const width = entries[0]?.contentRect.width ?? 0;
  emit('measure', estimateColumns(width, props.fontSize));
});

watch(() => props.fontSize, () => {
  const width = scrollComp.value?.scroller?.clientWidth ?? 0;
  emit('measure', estimateColumns(width, props.fontSize));
});

function resizeInput(): void {
  const node = inputRef.value;
  if (!node) return;
  node.style.height = 'auto';
  node.style.height = `${Math.min(MAX_INPUT_HEIGHT, node.scrollHeight)}px`;
}

function scrollToBottom(): void {
  const el = scrollComp.value?.scroller;
  if (!el) return;
  el.scrollTop = el.scrollHeight;
  following.value = true;
}

function onScroll(): void {
  const el = scrollComp.value?.scroller;
  if (!el) return;
  following.value = el.scrollHeight - el.scrollTop - el.clientHeight < FOLLOW_THRESHOLD;
}

// `.length`, not the array itself: `scrollback` is a ref whose value is
// mutated in place (`push`) far more often than it is replaced, and a watch
// source has to read something that actually changes on every append for
// the callback to fire on each one rather than only on a wholesale
// reassignment (`clearScrollback`'s `scrollback.value = []`).
watch(() => props.session.scrollback.value.length, () => {
  if (following.value) void nextTick(scrollToBottom);
});

onMounted(() => {
  resizeInput();
  scrollToBottom();
  inputRef.value?.focus({ preventScroll: true });
});

function onInput(event: Event): void {
  props.session.draft.value = (event.target as HTMLTextAreaElement).value;
  resizeInput();
}

function caretAtFirstLine(el: HTMLTextAreaElement): boolean {
  return !el.value.slice(0, el.selectionStart ?? 0).includes('\n');
}

function caretAtLastLine(el: HTMLTextAreaElement): boolean {
  return !el.value.slice(el.selectionEnd ?? el.value.length).includes('\n');
}

async function submit(): Promise<void> {
  if (props.session.running.value) return;
  const line = props.session.draft.value;
  completions.value = [];
  await props.session.run(line);
  void nextTick(resizeInput);
}

async function onTab(el: HTMLTextAreaElement): Promise<void> {
  const pos = el.selectionStart ?? props.session.draft.value.length;
  const result = await props.session.complete(props.session.draft.value, pos);
  if (result.items.length !== 1) {
    completions.value = result.items;
    return;
  }
  const item = result.items[0]!;
  const before = props.session.draft.value.slice(0, result.from);
  const after = props.session.draft.value.slice(pos);
  props.session.draft.value = `${before}${item.value}${after}`;
  completions.value = [];
  await nextTick();
  const caret = before.length + item.value.length;
  el.setSelectionRange(caret, caret);
  el.focus();
  resizeInput();
}

function onKeyDown(event: KeyboardEvent): void {
  const el = event.currentTarget as HTMLTextAreaElement;

  if (event.key === 'Tab') {
    event.preventDefault();
    void onTab(el);
    return;
  }
  completions.value = [];

  if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    void submit();
    return;
  }
  if (event.key === 'ArrowUp' && !event.shiftKey && caretAtFirstLine(el)) {
    event.preventDefault();
    props.session.recall('up');
    return;
  }
  if (event.key === 'ArrowDown' && !event.shiftKey && caretAtLastLine(el)) {
    event.preventDefault();
    props.session.recall('down');
    return;
  }
  if (event.key.toLowerCase() === 'l' && event.ctrlKey) {
    event.preventDefault();
    props.session.clearScrollback();
    return;
  }
  if ((event.key === 'Escape' || (event.key.toLowerCase() === 'c' && event.ctrlKey)) && props.session.running.value) {
    event.preventDefault();
    props.session.cancel();
  }
}

// mouseup rather than click: a drag that ends with the button still over the
// pane both mouseup-and-clicks the same element, so a plain click handler
// would refocus the input in the middle of the reader dragging out a
// selection, collapsing it back to a caret before the mouse has even come up.
function onPaneMouseUp(): void {
  if (window.getSelection()?.toString()) return;
  inputRef.value?.focus({ preventScroll: true });
}
</script>

<template>
  <div class="oa-terminal-pane" @mouseup="onPaneMouseUp">
    <OaScrollArea ref="scrollComp" wrap-class="oa-terminal-scroll-wrap" scroll-class="oa-terminal-scroll" @scroll.passive="onScroll">
      <div class="oa-terminal-blocks">
        <TerminalLine
          v-for="block in props.session.scrollback.value"
          :key="block.id"
          :block="block"
          :timestamps="props.timestamps"
        />
      </div>
    </OaScrollArea>

    <button
      v-if="!following"
      type="button"
      class="oa-btn oa-terminal-jump"
      @click="scrollToBottom"
    >{{ t('terminalJumpToBottom') }}</button>

    <div v-if="completions.length" class="oa-terminal-completions" role="list">
      <span v-for="item in completions" :key="item.value" class="oa-terminal-completion-item" role="listitem" :title="item.hint">{{ item.label }}</span>
    </div>

    <div class="oa-terminal-prompt">
      <span class="oa-terminal-prompt-glyph" aria-hidden="true">&gt;</span>
      <textarea
        ref="inputRef"
        class="oa-terminal-input"
        rows="1"
        spellcheck="false"
        autocapitalize="off"
        autocomplete="off"
        :aria-label="t('terminalInputLabel')"
        :value="props.session.draft.value"
        @input="onInput"
        @keydown="onKeyDown"
      />
      <span v-if="props.session.running.value" class="oa-terminal-spinner" :title="t('terminalRunning')" aria-hidden="true" />
      <OaIconButton
        v-if="props.session.running.value"
        class="oa-icon-btn oa-terminal-cancel"
        :label="t('terminalCancel')"
        @click="props.session.cancel()"
      >
        <IconStop :size="12" />
      </OaIconButton>
    </div>
  </div>
</template>
