<script setup lang="ts">
// Who uses what: the heaviest accounts down the side, the busiest models
// across the top, and in each cell what that account spent on that model.
//
// A table rather than a drawing, because every cell is a figure somebody may
// want to read exactly, and a table is what a screen reader can walk. The
// fill is only there so the eye finds the heavy cells before reading any.

import { computed } from 'vue';
import type { UsageBreakdown, UsageCell, UsageMatrix } from '@/admin/api';
import { t } from '@/composables/useI18n';
import { intensity } from './scale';

type CellMetric = 'requests' | 'total_tokens' | 'credits';

const props = defineProps<{
  matrix: UsageMatrix;
  users: readonly UsageBreakdown[];
  models: readonly UsageBreakdown[];
  metric: CellMetric;
  format: (value: number) => string;
}>();

const emit = defineEmits<{
  (event: 'user', key: string): void;
  (event: 'model', key: string): void;
}>();

const cells = computed(() => {
  const out = new Map<string, UsageCell>();
  for (const cell of props.matrix.cells) out.set(`${cell.row}\u0000${cell.col}`, cell);
  return out;
});
const peak = computed(() => Math.max(0, ...props.matrix.cells.map((cell) => cell[props.metric])));

function byKey(list: readonly UsageBreakdown[], key: string): UsageBreakdown | undefined {
  return list.find((row) => row.key === key);
}
function label(list: readonly UsageBreakdown[], key: string): string {
  const row = byKey(list, key);
  return row?.label || key;
}
function cell(user: string, model: string): number {
  return cells.value.get(`${user}\u0000${model}`)?.[props.metric] ?? 0;
}
function level(user: string, model: string): number {
  return intensity(cell(user, model), peak.value);
}
function fill(user: string, model: string): string | undefined {
  const amount = level(user, model);
  return amount > 0 ? `${Math.round(12 + amount * 88)}%` : undefined;
}
/** The row's total over every model, not only the ones shown across the top. */
function rowTotal(user: string): number {
  const row = byKey(props.users, user);
  if (!row) return 0;
  return props.metric === 'requests' ? row.requests : props.metric === 'total_tokens' ? row.total_tokens : row.credits;
}
</script>

<template>
  <div class="oa-matrix-wrap" tabindex="0" :aria-label="t('matrixTitle')">
    <table class="oa-matrix">
      <thead>
        <tr>
          <th scope="col" class="oa-matrix-corner">{{ t('colUser') }}</th>
          <th v-for="model in props.matrix.cols" :key="model" scope="col">
            <button type="button" class="oa-matrix-head" :title="t('boardDrill', { name: label(props.models, model) })" @click="emit('model', model)">
              <span>{{ label(props.models, model) }}</span>
              <small>{{ byKey(props.models, model)?.detail }}</small>
            </button>
          </th>
          <th scope="col" class="oa-matrix-total">{{ t('matrixAllModels') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="user in props.matrix.rows" :key="user">
          <th scope="row">
            <button type="button" class="oa-matrix-user" :title="t('boardDrill', { name: label(props.users, user) })" @click="emit('user', user)">
              <span class="oa-board-mark" aria-hidden="true">{{ Array.from(label(props.users, user))[0]?.toLocaleUpperCase() }}</span>
              <span>{{ label(props.users, user) }}</span>
            </button>
          </th>
          <td
            v-for="model in props.matrix.cols"
            :key="model"
            :class="{ filled: !!fill(user, model), strong: level(user, model) > 0.6 }"
            :style="fill(user, model) ? { '--oa-heat': fill(user, model) } : undefined"
            :title="`${label(props.users, user)} · ${label(props.models, model)} · ${props.format(cell(user, model))}`"
          >{{ cell(user, model) ? props.format(cell(user, model)) : '·' }}</td>
          <td class="oa-matrix-total">{{ props.format(rowTotal(user)) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
