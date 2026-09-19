<script setup lang="ts">
// The projects tab's contents: create, open, edit and delete, all as one
// list rather than a screen of its own — a project is a name and a brief,
// small enough that a side panel for it would be more chrome than content.

import { computed, nextTick, onMounted, ref } from 'vue';
import type { Project } from '@/api/projects';
import { ApiError } from '@/api/client';
import OaConfirmButton from '@/components/OaConfirmButton.vue';
import OaScrollArea from '@/components/OaScrollArea.vue';
import OaTextArea from '@/components/OaTextArea.vue';
import OaTextField from '@/components/OaTextField.vue';
import { t } from '@/composables/useI18n';
import { IconCheck, IconGear, IconPlus, IconTrash } from '@/icons';
import {
  createProject, deleteProject, loadProjects, projectList, projectMax,
  renameProject, updateProjectInstructions,
} from '@/stores/projects';
import { pendingProjectID, setProject } from '@/stores/workspace';
// Shared with the rest of the chat rather than a second banner of its own:
// a failed rename and a failed send are the same kind of interruption, and
// the reader is already watching this one spot for it.
import { flash } from './useChat';

onMounted(() => void loadProjects());

const atMax = computed(() => projectMax.value > 0 && projectList.value.length >= projectMax.value);
const busy = ref(false);

function reportFailure(error: unknown): void {
  flash.value = error instanceof ApiError ? error.message : t('failed');
}

// --- create ------------------------------------------------------------------

const creating = ref(false);
const createName = ref('');
const createInstructions = ref('');
const createNameField = ref<InstanceType<typeof OaTextField> | null>(null);

function toggleCreate(): void {
  creating.value = !creating.value;
  if (!creating.value) return;
  editingID.value = '';
  createName.value = '';
  createInstructions.value = '';
  void nextTick(() => createNameField.value?.focus());
}

async function submitCreate(): Promise<void> {
  const name = createName.value.trim();
  if (!name) return;
  busy.value = true;
  try {
    // Taken straight into work mode in the project just made: creating one
    // and then having to find and click it again would be the same gesture
    // twice for one intention.
    const record = await createProject(name, createInstructions.value.trim());
    creating.value = false;
    setProject(record.id);
  } catch (error) {
    reportFailure(error);
  } finally {
    busy.value = false;
  }
}

// --- edit: rename and the standing instructions share one form, the way
// KeysPanel treats an edit as the same row looked at more closely rather
// than a second column ---------------------------------------------------

const editingID = ref('');
const editName = ref('');
const editInstructions = ref('');
const editNameField = ref<InstanceType<typeof OaTextField> | null>(null);

function beginEdit(project: Project): void {
  creating.value = false;
  editingID.value = project.id;
  editName.value = project.name;
  editInstructions.value = project.instructions;
  void nextTick(() => editNameField.value?.focus());
}

function cancelEdit(): void {
  editingID.value = '';
}

async function submitEdit(project: Project): Promise<void> {
  const name = editName.value.trim();
  if (!name) return;
  busy.value = true;
  try {
    // Two PATCHes, not one, because the store exposes what the server does:
    // a name change and an instructions change are separate requests, and
    // sending the one that did not change would just be a no-op write.
    if (name !== project.name) await renameProject(project.id, name);
    if (editInstructions.value !== project.instructions) {
      await updateProjectInstructions(project.id, editInstructions.value);
    }
    editingID.value = '';
  } catch (error) {
    reportFailure(error);
  } finally {
    busy.value = false;
  }
}

async function remove(project: Project): Promise<void> {
  try {
    await deleteProject(project.id);
    // The instructions that just left are what "work" in this project meant;
    // work mode itself is not a fact about the project, so it stays.
    if (pendingProjectID.value === project.id) setProject('');
  } catch (error) {
    reportFailure(error);
  }
}

function choose(project: Project): void {
  setProject(project.id);
}
</script>

<template>
  <div class="ai-project-panel">
    <button
      type="button"
      class="ai-chat-new"
      :disabled="atMax && !creating"
      @click="toggleCreate"
    >
      <IconPlus :size="14" />
      <span>{{ t('newProject') }}</span>
    </button>

    <p v-if="atMax" class="ai-chat-list-empty">{{ t('projectsFull') }}</p>

    <div v-if="creating" class="ai-project-form">
      <OaTextField
        ref="createNameField"
        v-model="createName"
        :label="t('projectName')"
        :placeholder="t('projectNamePlaceholder')"
        :max-length="80"
      />
      <OaTextArea
        v-model="createInstructions"
        :label="t('projectInstructions')"
        :hint="t('projectInstructionsHint')"
        :rows="4"
      />
      <div class="oa-key-edit-actions">
        <button type="button" class="oa-btn" @click="creating = false">{{ t('cancel') }}</button>
        <button type="button" class="oa-btn primary" :disabled="busy || !createName.trim()" @click="submitCreate">
          {{ t('saveProject') }}
        </button>
      </div>
    </div>

    <OaScrollArea wrap-class="ai-chat-list-wrap" scroll-class="ai-chat-list">
      <p v-if="!projectList.length" class="ai-chat-list-empty">{{ t('noProjects') }}</p>
      <template v-for="project in projectList" :key="project.id">
        <div
          v-if="editingID !== project.id"
          class="ai-chat-list-item"
          :class="{ active: project.id === pendingProjectID }"
        >
          <button type="button" class="ai-chat-list-open" :title="t('openProject')" @click="choose(project)">
            <span class="ai-chat-list-title">{{ project.name }}</span>
            <span class="ai-project-meta">{{ project.conversations }} {{ t('projectConversations') }}</span>
          </button>
          <button
            type="button"
            class="ai-project-action-btn"
            :title="t('editProject')"
            :aria-label="t('editProject')"
            @click="beginEdit(project)"
          ><IconGear :size="13" /></button>
          <OaConfirmButton
            class="ai-project-action-btn danger"
            :resting-title="t('deleteProject')"
            :armed-title="t('deleteProjectConfirm')"
            @confirm="remove(project)"
          >
            <IconTrash :size="13" />
            <template #armed><IconCheck :size="13" /></template>
          </OaConfirmButton>
        </div>

        <div v-else class="ai-project-form">
          <OaTextField
            ref="editNameField"
            v-model="editName"
            :label="t('projectName')"
            :placeholder="t('projectNamePlaceholder')"
            :max-length="80"
          />
          <OaTextArea
            v-model="editInstructions"
            :label="t('projectInstructions')"
            :hint="t('projectInstructionsHint')"
            :rows="4"
          />
          <div class="oa-key-edit-actions">
            <button type="button" class="oa-btn" @click="cancelEdit">{{ t('cancel') }}</button>
            <button type="button" class="oa-btn primary" :disabled="busy || !editName.trim()" @click="submitEdit(project)">
              {{ t('saveProject') }}
            </button>
          </div>
        </div>
      </template>
    </OaScrollArea>
  </div>
</template>
