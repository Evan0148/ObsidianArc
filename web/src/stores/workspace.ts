// Which surface the composer is pointed at, and which project it is
// pointed into.
//
// Two values in one place for the same reason session.ts holds three: the
// rail, the composer, the greeting and the transcript all need to agree
// about what the reader is doing, and passing that down four component
// layers is how two of them end up disagreeing.
//
// Only the *pending* choice lives here — what the next new conversation
// will be. Once one exists it carries its own mode and project, read back
// from the server with the thread, because a conversation does not change
// character because the rail's toggle moved afterwards.

import { computed, ref, type ComputedRef, type Ref } from 'vue';

export type Mode = 'chat' | 'work';

const mode = ref<Mode>('chat');
const projectID = ref('');

/** What a new conversation would be opened as. */
export const pendingMode: Ref<Mode> = mode;

/** The project a new conversation would be opened in, empty for none. */
export const pendingProjectID: Ref<string> = projectID;

export const isWork: ComputedRef<boolean> = computed(() => mode.value === 'work');

/**
 * Points the composer at a surface.
 *
 * Leaving work mode drops the project with it: a project is a work thing,
 * and a chat that claimed to be in one would inherit a brief written for an
 * agent that is no longer there.
 */
export function setMode(next: Mode): void {
  mode.value = next;
  if (next !== 'work') {
    projectID.value = '';
  }
}

/**
 * Opens the composer into a project.
 *
 * A project is always work, so choosing one says so rather than leaving the
 * reader to set two controls for one intention.
 */
export function setProject(id: string): void {
  projectID.value = id;
  if (id) {
    mode.value = 'work';
  }
}
