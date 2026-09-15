<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { adminApi, type Account, type Role } from '@/admin/api';
import { ADMIN_PAGES } from '@/admin/permissions';
import OaBadge from '@/components/OaBadge.vue';
import OaCheckList from '@/components/OaCheckList.vue';
import OaPanel from '@/components/OaPanel.vue';
import OaSearchField from '@/components/OaSearchField.vue';
import OaSelectField from '@/components/OaSelectField.vue';
import OaTable from '@/components/OaTable.vue';
import type { Column, PageState } from '@/components/table-types';
import { t } from '@/composables/useI18n';
import { canAdmin, currentUser, isSuperAdmin } from '@/stores/session';
import { useAdminView } from './adminView';

type Candidate = Pick<Account, 'id' | 'username' | 'nickname' | 'role' | 'admin_permissions'>;
const view = useAdminView();
view.setTitle(t('manageAdministrators'), t('administratorsHint'));
const query = ref('');
const page = ref<PageState>({ page: 1, pageSize: 20 });
const total = ref(0);
const rows = ref<Candidate[]>([]);
const loading = ref(false);
const error = ref('');
const target = ref<Candidate | null>(null);
const role = ref<Role>('admin');
const permissions = ref<string[]>([]);
const saving = ref(false);
const panelError = ref('');
let request = 0;
let timer = 0;
const columns = computed<Column<Candidate>[]>(() => [
  { key: 'name', header: t('colAccount'), text: (row) => row.nickname || row.username },
  { key: 'username', header: t('username'), text: (row) => '@' + row.username },
  { key: 'role', header: t('colRole') },
]);
const editable = computed(() => isSuperAdmin.value || (target.value?.role !== 'super_admin' && target.value?.id !== currentUser.value?.id && (target.value?.admin_permissions ?? []).every(canAdmin)));
const choices = computed(() => ADMIN_PAGES.filter((entry) => canAdmin(entry.value)).map((entry) => ({ value: entry.value, label: t(entry.label) })));
function open(row: Candidate): void {
  target.value = row;
  role.value = row.role === 'user' ? 'admin' : row.role;
  permissions.value = [...(row.admin_permissions ?? [])];
  panelError.value = '';
}
async function load(): Promise<void> {
  const ticket = ++request;
  loading.value = true;
  error.value = '';
  try {
    const result = await adminApi.administrators(
      '?' + new URLSearchParams({ q: query.value, limit: String(page.value.pageSize), offset: String((page.value.page - 1) * page.value.pageSize) }));
    if (ticket !== request) return;
    rows.value = result.users ?? [];
    total.value = result.total;
  } catch (failure) { if (ticket === request) error.value = String(failure); }
  finally { if (ticket === request) loading.value = false; }
}
function changePage(next: PageState): void { page.value = next; void load(); }
watch(query, () => {
  ++request; page.value.page = 1;
  window.clearTimeout(timer);
  timer = window.setTimeout(() => void load(), 250);
});
async function save(): Promise<void> {
  if (!target.value || saving.value || !editable.value) return;
  saving.value = true;
  try {
    const result = await adminApi.updateAdministrator(target.value.id, { role: role.value, admin_permissions: role.value === 'admin' ? permissions.value : [] });
    if (result.user.id === currentUser.value?.id) currentUser.value = { ...currentUser.value, ...result.user };
    target.value = null;
    await load();
  } catch (failure) { panelError.value = String(failure); }
  finally { saving.value = false; }
}
void load();
onBeforeUnmount(() => { ++request; window.clearTimeout(timer); });
</script>

<template>
  <OaSearchField v-model="query" :label="t('searchUsers')" />
  <p v-if="error" class="oa-field-hint" role="alert">{{ error }}</p>
  <OaTable :columns="columns" :rows="rows" :empty="t('noAccountsMatch')" :pagination="{ ...page, total }" :busy="loading" selectable @select="open" @page="changePage">
    <template #cell-role="{ row }"><OaBadge>{{ t(row.role === 'super_admin' ? 'superAdmin' : row.role === 'admin' ? 'admin' : 'roleUser') }}</OaBadge></template>
  </OaTable>
  <OaPanel v-if="target" :title="target.nickname || target.username" :confirm-label="t('save')" :footer="editable" :busy="saving" :error="panelError" @close="target = null" @confirm="save">
    <p v-if="!editable" class="oa-field-hint" role="alert">{{ t('permissionDeniedHint') }}</p>
    <template v-else>
      <OaSelectField v-model="role" :label="t('role')" :options="[
        { value: 'user', label: t('roleUser') }, { value: 'admin', label: t('admin') },
        ...(isSuperAdmin ? [{ value: 'super_admin' as const, label: t('superAdmin') }] : []),
      ]" />
      <OaCheckList v-if="role === 'admin'" v-model="permissions" :label="t('adminPermissions')" :hint="t('adminPermissionsHint')" :items="choices" :empty-text="t('permissionDeniedTitle')" />
    </template>
  </OaPanel>
</template>
