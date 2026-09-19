import { computed, ref } from 'vue';

// Accept the submitted snapshot, so edits made while a save is in flight
// still appear as unsaved when that request finishes.
export function useSettingsDraft(collect: () => Record<string, string>) {
  const saved = ref<string | null>(null);
  const dirty = computed(() => saved.value !== null && saved.value !== JSON.stringify(collect()));
  const accept = (values = collect()): void => { saved.value = JSON.stringify(values); };
  return { dirty, accept };
}
