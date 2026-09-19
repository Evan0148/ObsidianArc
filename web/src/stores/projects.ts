// The account's projects, held in one place.
//
// Loaded once and kept in memory, same reasoning as session.ts: the rail and
// the composer both need the list, and a second `/api/projects` call every
// time a panel mounts would just be racing the first one. Plain refs and
// functions, no store library — a handful of values and actions do not need
// Pinia.

import { ref, type Ref } from 'vue';
import {
  createProject as apiCreateProject,
  deleteProject as apiDeleteProject,
  listProjects,
  updateProject as apiUpdateProject,
  type Project,
} from '@/api/projects';

const projects = ref<Project[]>([]);
const max = ref(0);
const loaded = ref(false);
// Shared by every caller so a rail and a composer mounting in the same tick
// await one request instead of two racing to set `loaded`.
let inFlight: Promise<void> | null = null;

export const projectList: Ref<Project[]> = projects;
/** The account-wide cap on how many projects it may hold. Zero until loaded. */
export const projectMax: Ref<number> = max;

/**
 * Fetches the list if it has not been fetched yet. Safe to call from every
 * screen that shows projects; only the first caller after boot (or after a
 * failed attempt) actually reaches the network.
 */
export function loadProjects(): Promise<void> {
  if (loaded.value) return Promise.resolve();
  if (!inFlight) {
    inFlight = listProjects()
      .then((result) => {
        projects.value = result.projects;
        max.value = result.max;
        loaded.value = true;
      })
      .finally(() => {
        inFlight = null;
      });
  }
  return inFlight;
}

export async function createProject(name: string, instructions = ''): Promise<Project> {
  const record = await apiCreateProject(name, instructions);
  // Newest first, matching the order the server itself returns on List
  // (most recently changed first).
  projects.value = [record, ...projects.value];
  return record;
}

export async function renameProject(id: string, name: string): Promise<Project> {
  const record = await apiUpdateProject(id, { name });
  replace(record);
  return record;
}

export async function updateProjectInstructions(id: string, instructions: string): Promise<Project> {
  const record = await apiUpdateProject(id, { instructions });
  replace(record);
  return record;
}

export async function deleteProject(id: string): Promise<void> {
  await apiDeleteProject(id);
  projects.value = projects.value.filter((project) => project.id !== id);
}

// The list is only ever touched with the server's own answer in hand, so a
// rejected request leaves it exactly as it was rather than half-applied.
function replace(record: Project): void {
  projects.value = projects.value.map((project) => (project.id === record.id ? record : project));
}
