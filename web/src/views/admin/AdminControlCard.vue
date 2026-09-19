<script setup lang="ts">
import type { Component } from 'vue';
defineProps<{ title: string; hint?: string; icon?: Component }>();
</script>

<template>
  <section class="oa-control-card">
    <header class="oa-control-card-head">
      <span v-if="icon" class="oa-control-card-icon"><component :is="icon" :size="18" /></span>
      <div><h3>{{ title }}</h3><p v-if="hint">{{ hint }}</p></div>
      <slot name="actions" />
    </header>
    <div class="oa-control-card-body oa-settings-panel"><slot /></div>
  </section>
</template>
