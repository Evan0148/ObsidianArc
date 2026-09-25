<script setup lang="ts">
// The dropdown the model chip, the bell and the account button all use.
//
// Only one menu is open at a time; opening a second closes the first, which
// is what stops two overlapping panels when a user clicks straight from one
// trigger to another.
//
// The contents are only in the document while it is open — the trigger slot
// gets `open` and `toggle`, the panel slot gets `close` — so a menu that has
// never been opened costs nothing and one that has been closed is not left
// holding a stale list.

import { onBeforeUnmount, ref } from 'vue';
import { onClickOutside, onKeyStroke } from '@vueuse/core';

const props = defineProps<{
  /** Extra class on the positioning wrapper, for alignment overrides. */
  groupClass?: string;
  menuClass?: string;
}>();

// Long enough for the transition in the stylesheet; a few ms over, so the
// last frame is not cut off.
const CLOSE_MS = 160;

const group = ref<HTMLElement | null>(null);
const panel = ref<HTMLElement | null>(null);

// How close to the screen's edge a menu may come.
const GUTTER = 8;

/**
 * Keeps an open menu on screen. A menu hangs from its trigger's edge, and on
 * a phone a trigger in the middle of the header — the bell — sits closer to
 * the left edge than the menu is wide, so the menu ran off the screen with
 * half its text. Narrowed to the screen first, then slid back inside it.
 *
 * `translate` rather than `transform`: the open and close animation owns
 * transform, and the two compose instead of one overwriting the other. The
 * menu is still scaled for that animation while this measures, so each edge
 * is measured as if the scale had already been undone.
 */
function fit(): void {
  const el = panel.value;
  if (!el) return;
  el.style.translate = '';
  el.style.maxWidth = '';
  el.style.minWidth = '';
  const room = document.documentElement.clientWidth - 2 * GUTTER;
  if (el.offsetWidth > room) {
    el.style.maxWidth = `${room}px`;
    el.style.minWidth = '0';
  }
  const rect = el.getBoundingClientRect();
  const slack = el.offsetWidth - rect.width;
  const left = rect.left - slack;
  const right = rect.right + slack;
  let shift = 0;
  if (left < GUTTER) shift = GUTTER - left;
  else if (right > GUTTER + room) shift = GUTTER + room - right;
  if (shift) el.style.translate = `${Math.round(shift)}px 0`;
}

// Open is tracked separately from `mounted` because closing keeps the menu in
// the document until its transition has run, and it is shut as far as anyone
// clicking is concerned well before then.
const open = ref(false);
const mounted = ref(false);
const shown = ref(false);
let hideTimer = 0;

function show(): void {
  for (const other of others) if (other !== close) other();
  others.add(close);
  window.clearTimeout(hideTimer);
  open.value = true;
  mounted.value = true;
  // One frame closed, so the transition has a state to move from — and the
  // menu is in the document by then, so it can be measured.
  requestAnimationFrame(() => {
    if (!open.value) return;
    fit();
    shown.value = true;
  });
}

function close(): void {
  if (!open.value) return;
  open.value = false;
  shown.value = false;
  others.delete(close);
  window.clearTimeout(hideTimer);
  hideTimer = window.setTimeout(() => {
    if (!open.value) mounted.value = false;
  }, CLOSE_MS);
}

function toggle(): void {
  if (open.value) close();
  else show();
}

onClickOutside(group, () => close());
onKeyStroke('Escape', () => close());

onBeforeUnmount(() => {
  window.clearTimeout(hideTimer);
  others.delete(close);
});

defineExpose({ isOpen: () => open.value, close, open: show });
</script>

<script lang="ts">
/** Every menu currently open. In practice never more than one. */
const others = new Set<() => void>();
</script>

<template>
  <div ref="group" class="oa-chip-group" :class="props.groupClass">
    <slot name="trigger" :open="open" :toggle="toggle" />
    <div v-if="mounted" ref="panel" class="oa-menu" :class="[props.menuClass, { open: shown }]">
      <slot :close="close" />
    </div>
  </div>
</template>
