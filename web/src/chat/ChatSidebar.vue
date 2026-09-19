<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import OaConfirmButton from '@/components/OaConfirmButton.vue';
import OaResizer from '@/components/OaResizer.vue';
import OaScrollArea from '@/components/OaScrollArea.vue';
import OaSearchField from '@/components/OaSearchField.vue';
import { matchesSearch } from '@/lib/search';
import { t } from '@/composables/useI18n';
import { usePanelHost } from '@/composables/usePanelHost';
import { IconCheck, IconPlus, IconTrash } from '@/icons';
import ProjectList from './ProjectList.vue';
import {
  activeID, canDelete, clearEverything, conversations, openConversation,
  pendingID, removeConversation, rename, startNewConversation,
} from './useChat';

// The rail's width drives its own collapsed margin as well as its size, so
// the handle writes the custom property on the row rather than a width here.
const row = usePanelHost();

// Which list the rail shows. Local and unpersisted: it is a view of the
// rail, not a fact about the account, so it does not need to survive a
// reload the way the pending mode and project in workspace.ts do.
const activeTab = ref<'history' | 'projects'>('history');
const query = ref('');
const filteredConversations = computed(() => conversations.value.filter((entry) =>
  matchesSearch(query.value, entry.title || t('newChat'))));

// A short rise on the row that just became current, so switching reads as a
// different conversation rather than the list quietly repainting.
const switched = ref(false);
let timer = 0;
watch(activeID, () => {
  switched.value = true;
  window.clearTimeout(timer);
  timer = window.setTimeout(() => { switched.value = false; }, 400);
});
</script>

<template>
  <div class="ai-chat-sidebar">
    <div class="ai-chat-sidebar-head">
      <!-- Replaces the old static "History" label: the tab a reader is on
           already says what the list below is, so the label and the switch
           are one control instead of two that could disagree. -->
      <div class="ai-sidebar-tabs" role="tablist">
        <button
          type="button"
          class="ai-sidebar-tab"
          :class="{ active: activeTab === 'history' }"
          role="tab"
          :aria-selected="activeTab === 'history'"
          @click="activeTab = 'history'"
        >{{ t('tabHistory') }}</button>
        <button
          type="button"
          class="ai-sidebar-tab"
          :class="{ active: activeTab === 'projects' }"
          role="tab"
          :aria-selected="activeTab === 'projects'"
          @click="activeTab = 'projects'"
        >{{ t('tabProjects') }}</button>
      </div>
      <button v-if="activeTab === 'history'" type="button" class="ai-chat-new" @click="startNewConversation">
        <IconPlus :size="14" />
        <span>{{ t('newChat') }}</span>
      </button>
    </div>

    <template v-if="activeTab === 'history'">
      <OaSearchField v-model="query" class="oa-history-search" :label="t('searchHistory')" />

      <OaScrollArea wrap-class="ai-chat-list-wrap" scroll-class="ai-chat-list">
        <p v-if="!conversations.length" class="ai-chat-list-empty">{{ t('noHistory') }}</p>
        <p v-else-if="!filteredConversations.length" class="ai-chat-list-empty" role="status">{{ t('noSearchResults') }}</p>
        <div
          v-for="conversation in filteredConversations"
          :key="conversation.id"
          class="ai-chat-list-item"
          :class="{
            active: conversation.id === activeID,
            switched: conversation.id === activeID && switched,
          }"
        >
          <button
            type="button"
            class="ai-chat-list-open"
            @click="openConversation(conversation.id)"
            @dblclick="rename(conversation)"
          >
            <span class="ai-chat-list-title">{{ conversation.title || t('newChat') }}</span>
          </button>
          <!-- The row that is still being written into, so a composer that is
               busy while the reader is somewhere else has a visible reason. -->
          <span
            v-if="conversation.id === pendingID"
            class="ai-chat-list-live"
            :title="t('thinking')"
          ><span class="ai-chat-spinner" /></span>
          <OaConfirmButton
            v-if="canDelete"
            class="ai-chat-list-delete"
            :resting-title="t('deleteChat')"
            :armed-title="t('confirmDelete')"
            @confirm="removeConversation(conversation)"
          >
            <IconTrash :size="13" />
            <template #armed><IconCheck :size="13" /></template>
          </OaConfirmButton>
        </div>
      </OaScrollArea>

      <div class="ai-chat-sidebar-foot" :hidden="conversations.length === 0">
        <OaConfirmButton
          v-if="canDelete"
          class="ai-chat-clear-all"
          :armed-label="t('clearAllConfirm')"
          :armed-title="t('confirmClearAll')"
          @confirm="clearEverything"
        >
          <IconTrash :size="12" />
          <span>{{ t('clearAll') }}</span>
        </OaConfirmButton>
      </div>
    </template>

    <ProjectList v-else />

    <OaResizer
      edge="right"
      css-variable="--ai-rail-width"
      :style-target="row"
      storage-key="obsidian-arc-rail-width"
      :min="190"
      :max="460"
      :fallback="260"
      :label="t('resizeRail')"
    />
  </div>
</template>
