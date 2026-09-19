import type { Component } from 'vue';
import type { StringKey } from '@/composables/useI18n';

export interface WorkbenchGroup {
  id: string;
  label: StringKey;
  hint: StringKey;
  icon: Component;
  sections: string[];
}
