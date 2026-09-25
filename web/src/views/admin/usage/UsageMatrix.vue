<script setup lang="ts">
// Who uses what: the heaviest accounts down the side, the busiest models
// across the top, and in each cell what that account spent on that model.
//
// A table rather than a drawing, because every cell is a figure somebody may
// want to read exactly, and a table is what a screen reader can walk. The
// fill is only there so the eye finds the heavy cells before reading any.

import { computed } from 'vue';
import type { UsageBreakdown, UsageCell, UsageMatrix } from '@/admin/api';
import { isMasked, maskBilling, maskProvider, maskUser } from '@/admin/safeMode';
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
// Safe mode, applied to everything this table prints, the tooltips too: a
// cell's title is read by hovering, and hovering is what a screen recording
// catches. The fill stays — it shows where the weight is, not what it is.
function userName(key: string): string {
  return maskUser(label(props.users, key));
}
function userMark(key: string): string {
  return isMasked('users') ? '*' : Array.from(label(props.users, key))[0]?.toLocaleUpperCase() ?? '·';
}
function modelDetail(key: string): string {
  return maskProvider(byKey(props.models, key)?.detail ?? '');
}
/** A request count is not spending; tokens and credits are. */
function figure(value: number): string {
  const text = props.format(value);
  return props.metric === 'requests' ? text : maskBilling(text);
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
              <small>{{ modelDetail(model) }}</small>
            </button>
          </th>
          <th scope="col" class="oa-matrix-total">{{ t('matrixAllModels') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="user in props.matrix.rows" :key="user">
          <th scope="row">
            <button type="button" class="oa-matrix-user" :title="t('boardDrill', { name: userName(user) })" @click="emit('user', user)">
              <span class="oa-board-mark" aria-hidden="true">{{ userMark(user) }}</span>
              <span>{{ userName(user) }}</span>
            </button>
          </th>
          <td
            v-for="model in props.matrix.cols"
            :key="model"
            :class="{ filled: !!fill(user, model), strong: level(user, model) > 0.6 }"
            :style="fill(user, model) ? { '--oa-heat': fill(user, model) } : undefined"
            :title="`${userName(user)} · ${label(props.models, model)} · ${figure(cell(user, model))}`"
          >{{ cell(user, model) ? figure(cell(user, model)) : '·' }}</td>
          <td class="oa-matrix-total">{{ figure(rowTotal(user)) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
