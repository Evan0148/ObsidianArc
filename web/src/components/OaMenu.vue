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

import { nextTick, onBeforeUnmount, ref } from 'vue';
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
  const room = document.documentElement.clientWidth - 2 * GUTTER;
  const menuWidth = el.offsetWidth;
  if (menuWidth > room) {
    el.style.maxWidth = `${room}px`;
    el.style.minWidth = '0';
  } else if (el.style.maxWidth) {
    el.style.maxWidth = '';
    el.style.minWidth = '';
  }

  // Calculate unscaled menu screen bounds from the trigger group anchor.
  // This avoids touching el.style.transform which causes synchronous style
  // invalidation and layout thrashing (frame rate jank) right before animation starts.
  const g = group.value;
  const viewWidth = document.documentElement.clientWidth;
  let shift = 0;

  if (g) {
    const gRect = g.getBoundingClientRect();
    const isLeft = el.classList.contains('oa-menu-left');
    const unscaledLeft = isLeft ? gRect.left : gRect.right - menuWidth;
    const unscaledRight = isLeft ? gRect.left + menuWidth : gRect.right;

    if (unscaledLeft < GUTTER) {
      shift = GUTTER - unscaledLeft;
    } else if (unscaledRight > viewWidth - GUTTER) {
      shift = (viewWidth - GUTTER) - unscaledRight;
    }
  } else {
    const prevTransform = el.style.transform;
    el.style.transform = 'none';
    const rect = el.getBoundingClientRect();
    el.style.transform = prevTransform;

    if (rect.left < GUTTER) {
      shift = GUTTER - rect.left;
    } else if (rect.right > viewWidth - GUTTER) {
      shift = (viewWidth - GUTTER) - rect.right;
    }
  }

  const newTranslate = shift ? `${Math.round(shift)}px 0` : '';
  if (el.style.translate !== newTranslate) {
    el.style.translate = newTranslate;
  }
}

// Open is tracked separately from `mounted` because closing keeps the menu in
// the document until its transition has run, and it is shut as far as anyone
// clicking is concerned well before then.
const open = ref(false);
const mounted = ref(false);
const shown = ref(false);
let hideTimer = 0;

async function show(): Promise<void> {
  for (const other of others) if (other !== close) other();
  others.add(close);
  window.clearTimeout(hideTimer);
  open.value = true;
  mounted.value = true;

  // Let Vue mount the element into the DOM first
  await nextTick();
  if (!open.value) return;

  // Resolve bounds/shift before transition starts to avoid layout thrashing during animation
  fit();

  // Commit transform in next animation frame for silky smooth hardware-composited 60/120fps
  requestAnimationFrame(() => {
    if (!open.value) return;
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
