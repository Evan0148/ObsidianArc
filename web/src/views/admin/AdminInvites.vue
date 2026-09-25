<script setup lang="ts">
// Invite codes: who may join without an invitation, and who let everyone
// else in.
//
// Three things share this screen because they are one subject read from
// three angles: the registration policy (open, invite-only or closed), the
// codes that policy is enforced through — an operator's batch, a partner's
// named link, or the code every account may carry for its own referrals —
// and what a qualifying invite is worth to the person who sent it.

import { computed, onMounted, ref, watch } from 'vue';
import { useDebounceFn } from '@vueuse/core';
import { adminApi, type Group, type InviteCode, type InviteStats, type InviteUse } from '@/admin/api';
import { fetchSite } from '@/api/auth';
import { ApiError } from '@/api/client';
import { copyToClipboard } from '@/chat/markdown';
import AdminControlCard from './AdminControlCard.vue';
import OaBadge from '@/components/OaBadge.vue';
import OaCellStack from '@/components/OaCellStack.vue';
import OaFormSection from '@/components/OaFormSection.vue';
import OaNumberField from '@/components/OaNumberField.vue';
import OaPanel from '@/components/OaPanel.vue';
import OaSelect from '@/components/OaSelect.vue';
import OaSelectField from '@/components/OaSelectField.vue';
import OaStatGrid from '@/components/OaStatGrid.vue';
import OaSwitchField from '@/components/OaSwitchField.vue';
import OaTable from '@/components/OaTable.vue';
import OaTextField from '@/components/OaTextField.vue';
import type { Column, PageState } from '@/components/table-types';
import type { Stat } from '@/components/stat';
import { t } from '@/composables/useI18n';
import { IconChart, IconUsers } from '@/icons';
import { compactNumber, relativeTime } from '@/lib/format';
import { maskCredential, maskUser } from '@/admin/safeMode';
import { site } from '@/stores/session';
import AdminFailure from './AdminFailure.vue';
import { useAdminView } from './adminView';

const view = useAdminView();
view.setTitle(t('navInvites'), t('invitesSubtitle'));

/** An 8-character generated code reads as XXXX-XXXX; anything else — a
 *  partner's own name for their link — is shown exactly as stored. */
function formatCode(code: string): string {
  return code.length === 8 ? `${code.slice(0, 4)}-${code.slice(4)}` : code;
}

function inviteLink(code: string): string {
  return `${window.location.origin}/register?invite=${code}`;
}

// --- settings: the registration policy and what an invite is worth --------------

const settingsError = ref('');
const settingsLoaded = ref(false);
const savingBusy = ref(false);
const saveLabel = ref('');
const saveFlash = ref('');
const groups = ref<Pick<Group, 'id' | 'name'>[]>([]);

const form = ref({
  mode: 'open',
  userEnabled: false,
  userLimit: 10 as number | null,
  rewardCards: 0 as number | null,
  rewardCardDays: 30 as number | null,
});

/** Only this page's keys — the settings endpoint leaves every other screen's
 *  fields alone, which is what lets this card and Security's own registration
 *  switch both write `registration.enabled` without either reverting it. */
function collect(): Record<string, string> {
  return {
    'registration.enabled': String(form.value.mode !== 'closed'),
    'invites.required': String(form.value.mode === 'invite'),
    'invites.user_enabled': String(form.value.userEnabled),
    'invites.user_limit': String(form.value.userLimit ?? 10),
    'invites.reward_cards': String(form.value.rewardCards ?? 0),
    'invites.reward_card_days': String(form.value.rewardCardDays ?? 30),
  };
}

const dirty = computed(() => saved.value !== null && saved.value !== JSON.stringify(collect()));
const saved = ref<string | null>(null);
function accept(values = collect()): void { saved.value = JSON.stringify(values); }

async function loadSettings(): Promise<void> {
  settingsError.value = '';
  try {
    const [data, groupOptions] = await Promise.all([adminApi.settings(), adminApi.groupOptions()]);
    const values = data.settings;
    groups.value = groupOptions.groups ?? [];
    const enabled = values['registration.enabled'] === 'true';
    const required = values['invites.required'] === 'true';
    form.value = {
      mode: !enabled ? 'closed' : required ? 'invite' : 'open',
      userEnabled: values['invites.user_enabled'] === 'true',
      userLimit: Number(values['invites.user_limit'] ?? 10),
      rewardCards: Number(values['invites.reward_cards'] ?? 0),
      rewardCardDays: Number(values['invites.reward_card_days'] ?? 30),
    };
    accept();
  } catch (failure) {
    settingsError.value = failure instanceof ApiError ? failure.message : String(failure);
  } finally {
    settingsLoaded.value = true;
  }
}

async function saveSettings(): Promise<void> {
  if (!settingsLoaded.value || settingsError.value || savingBusy.value) return;
  const values = collect();
  savingBusy.value = true;
  saveLabel.value = t('saving');
  saveFlash.value = '';
  try {
    await adminApi.saveSettings(values);
    accept(values);
    try {
      // Registration mode is the one setting here a signed-out visitor reads
      // straight off this response, so the change should be visible without
      // a reload the moment it is saved.
      site.value = await fetchSite();
    } catch {
      // The setting itself is already saved; a failed public refresh is not
      // worth reporting as one.
    }
    saveLabel.value = t('saved');
    window.setTimeout(() => { saveLabel.value = ''; }, 1500);
  } catch (failure) {
    saveFlash.value = failure instanceof ApiError ? failure.message : String(failure);
    saveLabel.value = '';
  } finally {
    savingBusy.value = false;
  }
}

// --- stats -----------------------------------------------------------------------

const stats = ref<InviteStats | null>(null);
const statsError = ref('');

async function loadStats(): Promise<void> {
  statsError.value = '';
  try {
    stats.value = await adminApi.inviteStats();
  } catch (failure) {
    statsError.value = failure instanceof ApiError ? failure.message : String(failure);
  }
}

const statGridItems = computed<Stat[]>(() => (stats.value ? [
  { label: t('statActiveCodes'), value: compactNumber(stats.value.active) },
  { label: t('statInvitesTotal'), value: compactNumber(stats.value.uses_total) },
  { label: t('statInvites7d'), value: compactNumber(stats.value.uses_7d) },
] : []));

// --- the code list: filtered, searched, paged -------------------------------------

const codes = ref<InviteCode[]>([]);
const total = ref(0);
const paging = ref<PageState>({ page: 1, pageSize: 20 });
const listing = ref(false);
const listError = ref('');
let listRequest = 0;

const filters = ref({ kind: 'all', status: 'all', q: '' });

function changePage(next: PageState): void { paging.value = next; void list(); }
function filterList(): void { paging.value.page = 1; void list(); }
const debouncedList = useDebounceFn(() => void list(), 250);
watch(() => filters.value.q, () => { paging.value.page = 1; void debouncedList(); });

async function list(): Promise<void> {
  const ticket = ++listRequest;
  listing.value = true;
  listError.value = '';
  const query = new URLSearchParams({
    limit: String(paging.value.pageSize),
    offset: String((paging.value.page - 1) * paging.value.pageSize),
  });
  if (filters.value.kind !== 'all') query.set('kind', filters.value.kind);
  if (filters.value.status !== 'all') query.set('status', filters.value.status);
  if (filters.value.q.trim()) query.set('q', filters.value.q.trim());
  try {
    const result = await adminApi.invites(`?${query}`);
    if (ticket !== listRequest) return;
    if (result.total > 0 && (paging.value.page - 1) * paging.value.pageSize >= result.total) {
      paging.value.page = Math.ceil(result.total / paging.value.pageSize);
      await list(); return;
    }
    codes.value = result.codes ?? [];
    total.value = result.total;
  } catch (failure) {
    if (ticket !== listRequest) return;
    listError.value = failure instanceof ApiError ? failure.message : String(failure);
  } finally {
    if (ticket === listRequest) listing.value = false;
  }
}

function ownerTitle(row: InviteCode): string {
  return row.owner_id ? maskUser(row.owner_nickname || row.owner_username) : t('inviteOwnerAdmin');
}
function ownerSub(row: InviteCode): string | undefined {
  return row.owner_id ? `@${maskUser(row.owner_username)}` : undefined;
}

/** "3–7 days", a fixed "30 days", or "Permanent" for a fixed length of zero.
 *  A code with no group carries none of this. */
function daysLabel(row: InviteCode): string {
  if (!row.group_id) return '—';
  if (row.group_days_max > 0) return t('invitesDaysRange', { from: row.group_days, to: row.group_days_max });
  return row.group_days === 0 ? t('invitesPermanent') : t('nDays', { count: row.group_days });
}

function statusLabel(status: InviteCode['status']): string {
  if (status === 'active') return t('inviteStatusActive');
  if (status === 'used_up') return t('inviteStatusUsedUp');
  if (status === 'expired') return t('inviteStatusExpired');
  return t('inviteStatusRevoked');
}
function statusTone(status: InviteCode['status']): 'default' | 'muted' | 'danger' {
  if (status === 'active') return 'default';
  if (status === 'used_up') return 'muted';
  return 'danger';
}

const columns = computed<Array<Column<InviteCode>>>(() => [
  { key: 'code', header: t('colCode') },
  { key: 'owner', header: t('colOwner'), secondary: true, width: '150px' },
  { key: 'group', header: t('colGroup'), secondary: true, width: '150px' },
  {
    key: 'uses', header: t('colUses'), numeric: true, width: '80px',
    text: (row) => `${row.uses} / ${row.max_uses || '∞'}`,
  },
  {
    key: 'expires', header: t('colExpires'), secondary: true, width: '110px',
    text: (row) => (row.expires_at ? relativeTime(row.expires_at) : '—'),
  },
  { key: 'status', header: t('colState'), width: '90px' },
]);

// --- the generate / detail panel ---------------------------------------------------
//
// One panel, three faces — the same shape AdminCodes uses for redemption
// codes: a form that creates a batch, the batch it just made (copy it now,
// nowhere else lists it together), or an existing code with who has used it.
// `footer` and `confirmable` are set separately rather than tied to
// `creating` alone, because a code being viewed still needs its one action —
// revoking it — in that footer.

const panelOpen = ref(false);
const existing = ref<InviteCode | null>(null);
const minted = ref<InviteCode[] | null>(null);
const busy = ref(false);
const panelError = ref('');
const copyFlash = ref('');

const uses = ref<InviteUse[]>([]);
const usesLoading = ref(false);
const usesError = ref('');

const creating = computed(() => existing.value === null && minted.value === null);

const genForm = ref({
  count: 1 as number | null,
  code: '',
  maxUses: 1 as number | null,
  expiresDays: null as number | null,
  groupId: '',
  daysFrom: 0 as number | null,
  daysTo: null as number | null,
  note: '',
});

function openGenerate(): void {
  existing.value = null;
  minted.value = null;
  panelError.value = '';
  copyFlash.value = '';
  uses.value = [];
  usesError.value = '';
  genForm.value = { count: 1, code: '', maxUses: 1, expiresDays: null, groupId: '', daysFrom: 0, daysTo: null, note: '' };
  panelOpen.value = true;
}

function openExisting(row: InviteCode): void {
  minted.value = null;
  existing.value = row;
  panelError.value = '';
  copyFlash.value = '';
  uses.value = [];
  usesError.value = '';
  usesLoading.value = true;
  panelOpen.value = true;
  void loadUses(row.id);
}

async function loadUses(id: string): Promise<void> {
  try {
    const result = await adminApi.inviteUses(id);
    if (existing.value?.id === id) uses.value = result.uses ?? [];
  } catch (failure) {
    if (existing.value?.id === id) usesError.value = failure instanceof ApiError ? failure.message : String(failure);
  } finally {
    if (existing.value?.id === id) usesLoading.value = false;
  }
}

/** "@nickname · Rewarded", or the reason it was not — folded into the
 *  subtitle rather than a badge, so this list stays the same two-column row
 *  every other card list on this screen already is. */
function useSub(use: InviteUse): string {
  const base = `@${maskUser(use.username)}`;
  if (use.rewarded_at) return `${base} · ${t('inviteRewardedBadge')}`;
  if (use.reward_skipped === 'same_ip') return `${base} · ${t('inviteSkippedSameIP')}`;
  if (use.reward_skipped === 'limit') return `${base} · ${t('inviteSkippedLimit')}`;
  if (use.reward_skipped === 'disabled') return `${base} · ${t('inviteSkippedDisabled')}`;
  return base;
}

async function create(): Promise<void> {
  const count = genForm.value.count ?? 1;
  const days = genForm.value.expiresDays;
  const groupId = genForm.value.groupId;
  const from = genForm.value.daysFrom ?? 0;
  const to = genForm.value.daysTo ?? 0;
  busy.value = true;
  panelError.value = '';
  try {
    const { codes: created } = await adminApi.createInvites({
      count,
      // A batch is generated; naming one is only offered for a single code.
      code: count > 1 ? '' : genForm.value.code.trim(),
      max_uses: genForm.value.maxUses ?? 0,
      // Zero is "never", which is what an empty field means here.
      expires_at: days && days > 0 ? Date.now() + days * 24 * 3600 * 1000 : 0,
      group_id: groupId,
      group_days: groupId ? from : 0,
      // A "to" no higher than "from" is a fixed length, not a range.
      group_days_max: groupId && to > from ? to : 0,
      note: genForm.value.note.trim(),
    });
    minted.value = created;
    void list();
    void loadStats();
  } catch (failure) {
    panelError.value = failure instanceof ApiError ? failure.message : String(failure);
  } finally {
    busy.value = false;
  }
}

async function revoke(): Promise<void> {
  const row = existing.value;
  if (!row) return;
  busy.value = true;
  try {
    const { code: updated } = await adminApi.revokeInvite(row.id);
    existing.value = updated;
    void list();
    void loadStats();
  } catch (failure) {
    panelError.value = failure instanceof ApiError ? failure.message : String(failure);
  } finally {
    busy.value = false;
  }
}

function copyCode(code: string): void {
  void copyToClipboard(formatCode(code)).then((ok) => { copyFlash.value = ok ? t('copied') : t('copyFailed'); });
}
function copyLink(code: string): void {
  void copyToClipboard(inviteLink(code)).then((ok) => { copyFlash.value = ok ? t('copied') : t('copyFailed'); });
}
function copyAllCodes(): void {
  const lines = (minted.value ?? []).map((entry) => formatCode(entry.code)).join('\n');
  void copyToClipboard(lines).then((ok) => { copyFlash.value = ok ? t('copied') : t('copyFailed'); });
}

onMounted(() => {
  void loadSettings();
  void loadStats();
  void list();
});
</script>

<template>
  <Teleport :to="view.actionsHost">
    <span v-if="settingsLoaded && !settingsError" class="oa-control-save-state" :class="{ dirty }" role="status">
      <span class="oa-dashboard-dot" />{{ dirty ? t('controlUnsaved') : t('controlSaved') }}
    </span>
    <button type="button" class="oa-btn" :disabled="savingBusy || !settingsLoaded || !!settingsError" @click="saveSettings">
      {{ saveLabel || t('save') }}
    </button>
    <button id="generateInvites" type="button" class="oa-btn primary" @click="openGenerate">{{ t('generateInvites') }}</button>
  </Teleport>

  <AdminFailure v-if="settingsError" :message="settingsError" @retry="loadSettings" />
  <p v-else-if="!settingsLoaded" class="oa-table-empty">{{ t('loading') }}</p>

  <template v-else>
    <AdminControlCard id="secInviteSettings" :title="t('secInviteSettings')" :hint="t('secInviteSettingsHint')" :icon="IconUsers">
      <OaSelectField
        v-model="form.mode"
        :label="t('registrationMode')"
        :hint="t('registrationModeHint')"
        :options="[
          { value: 'open', label: t('registrationModeOpen') },
          { value: 'invite', label: t('registrationModeInvite') },
          { value: 'closed', label: t('registrationModeClosed') },
        ]"
      />
      <OaSwitchField v-model="form.userEnabled" :label="t('userInvitesEnabled')" :hint="t('userInvitesEnabledHint')" />
      <template v-if="form.userEnabled">
        <OaNumberField v-model="form.userLimit" :label="t('userInviteLimit')" :min="0" :max="10000" :hint="t('userInviteLimitHint')" />
        <OaNumberField v-model="form.rewardCards" :label="t('inviteRewardCards')" :min="0" :max="100" :hint="t('inviteRewardCardsHint')" />
        <OaNumberField
          v-if="(form.rewardCards ?? 0) > 0"
          v-model="form.rewardCardDays"
          :label="t('inviteRewardCardDays')"
          :min="1"
          :max="3650"
        />
      </template>
    </AdminControlCard>

    <AdminControlCard id="secInviteStats" :title="t('secInviteStats')" :icon="IconChart">
      <p v-if="statsError" class="oa-field-hint">{{ statsError }}</p>
      <p v-else-if="!stats" class="oa-field-hint">{{ t('loading') }}</p>
      <template v-else>
        <OaStatGrid :stats="statGridItems" />
        <OaFormSection :title="t('topInviters')" />
        <p v-if="!stats.top_inviters.length" class="oa-field-hint">{{ t('noTopInviters') }}</p>
        <div v-else class="oa-card-list">
          <div v-for="row in stats.top_inviters" :key="row.user_id" class="oa-card-row">
            <OaCellStack :title="maskUser(row.nickname || row.username)" :sub="`@${maskUser(row.username)}`" />
            <span class="oa-card-expiry">{{ t('inviteInviterSummary', { invites: row.invites, rewarded: row.rewarded }) }}</span>
          </div>
        </div>
      </template>
    </AdminControlCard>

    <div id="secInviteCodes" class="oa-filters">
      <input v-model="filters.q" type="search" :placeholder="t('searchInvites')">
      <OaSelect
        v-model="filters.kind"
        class="oa-filter-select"
        :choices="[
          { value: 'all', label: t('inviteKindAll') },
          { value: 'admin', label: t('inviteKindAdmin') },
          { value: 'user', label: t('inviteKindUser') },
        ]"
        @update:model-value="filterList"
      />
      <OaSelect
        v-model="filters.status"
        class="oa-filter-select"
        :choices="[
          { value: 'all', label: t('inviteStatusAll') },
          { value: 'active', label: t('inviteStatusActive') },
          { value: 'used_up', label: t('inviteStatusUsedUp') },
          { value: 'expired', label: t('inviteStatusExpired') },
          { value: 'revoked', label: t('inviteStatusRevoked') },
        ]"
        @update:model-value="filterList"
      />
    </div>

    <p v-if="listing" class="oa-table-empty">{{ t('loading') }}</p>
    <p v-if="listError" class="oa-table-empty">{{ listError }}</p>
    <OaTable
      :pagination="{ ...paging, total }"
      :busy="listing"
      @page="changePage"
      :columns="columns"
      :rows="codes"
      :empty="t('noInvites')"
      :muted="(row) => row.status !== 'active'"
      selectable
      @select="openExisting($event)"
    >
      <template #cell-code="{ row }">
        <OaCellStack :title="maskCredential(formatCode(row.code))" :sub="row.note || undefined" monospace />
      </template>
      <template #cell-owner="{ row }">
        <OaCellStack :title="ownerTitle(row)" :sub="ownerSub(row)" />
      </template>
      <template #cell-group="{ row }">
        <OaCellStack :title="row.group_id ? (row.group_name || '—') : t('inviteGroupNone')" :sub="row.group_id ? daysLabel(row) : undefined" />
      </template>
      <template #cell-status="{ row }">
        <OaBadge :tone="statusTone(row.status)">{{ statusLabel(row.status) }}</OaBadge>
      </template>
    </OaTable>

    <p v-if="saveFlash" class="oa-drawer-flash visible oa-control-flash" role="alert">{{ saveFlash }}</p>
  </template>

  <OaPanel
    v-if="panelOpen"
    :title="minted ? t('codesMinted', { count: minted.length }) : creating ? t('secGenerateInvites') : maskCredential(formatCode(existing!.code))"
    :footer="!minted"
    :confirmable="creating"
    :confirm-label="t('generateInvites')"
    :destructive-label="existing && existing.status !== 'revoked' ? t('revokeLabel') : undefined"
    :destructive-confirm="existing ? t('confirmRevokeInvite', { code: maskCredential(formatCode(existing.code)) }) : undefined"
    :busy="busy"
    :error="panelError"
    @close="panelOpen = false"
    @confirm="create"
    @destructive="revoke"
  >
    <!-- The batch, once, with a way to take it away in one piece. -->
    <template v-if="minted">
      <p class="oa-field-hint">{{ t('codesMintedHint') }}</p>
      <button type="button" class="oa-btn primary" @click="copyAllCodes">{{ copyFlash || t('copyAll') }}</button>
      <div class="oa-card-list">
        <div v-for="entry in minted" :key="entry.id" class="oa-card-row">
          <OaCellStack :title="maskCredential(formatCode(entry.code))" monospace />
          <div class="oa-invite-actions">
            <button type="button" class="oa-btn" @click="copyCode(entry.code)">{{ t('copyCode') }}</button>
            <button type="button" class="oa-btn" @click="copyLink(entry.code)">{{ t('copyLink') }}</button>
          </div>
        </div>
      </div>
    </template>

    <template v-else-if="!creating">
      <p class="oa-field-hint">
        {{ t('colUses') }}: {{ existing!.uses }} / {{ existing!.max_uses || '∞' }}
        <template v-if="existing!.group_id"> · {{ existing!.group_name || t('inviteGroupNone') }}, {{ daysLabel(existing!) }}</template>
      </p>
      <div class="oa-invite-actions">
        <button type="button" class="oa-btn" @click="copyCode(existing!.code)">{{ copyFlash || t('copyCode') }}</button>
        <button type="button" class="oa-btn" @click="copyLink(existing!.code)">{{ t('copyLink') }}</button>
      </div>
      <OaFormSection :title="t('inviteUsedBy')" />
      <p v-if="usesLoading" class="oa-field-hint">{{ t('loading') }}</p>
      <p v-else-if="usesError" class="oa-field-hint">{{ usesError }}</p>
      <p v-else-if="!uses.length" class="oa-field-hint">{{ t('inviteNoUses') }}</p>
      <div v-else class="oa-card-list">
        <div v-for="use in uses" :key="use.user_id" class="oa-card-row">
          <OaCellStack :title="maskUser(use.nickname || use.username)" :sub="useSub(use)" />
          <span class="oa-card-expiry">{{ relativeTime(use.created_at) }}</span>
        </div>
      </div>
    </template>

    <template v-else>
      <OaNumberField v-model="genForm.count" :label="t('inviteCount')" :min="1" :max="500" :hint="t('inviteCountHint')" />
      <!-- A batch is generated, so there is nothing to name — the field is
           for the one case that is naming a single, memorable link. -->
      <OaTextField
        v-if="(genForm.count ?? 1) <= 1"
        v-model="genForm.code"
        :label="t('inviteCustomCode')"
        placeholder="PARTNERX"
        :hint="t('inviteCustomCodeHint')"
        monospace
      />
      <OaNumberField v-model="genForm.maxUses" :label="t('inviteMaxUses')" :min="0" :max="100000" :hint="t('inviteMaxUsesHint')" />
      <OaNumberField
        v-model="genForm.expiresDays"
        :label="t('inviteExpiresDays')"
        :min="0"
        :placeholder="t('noLimit')"
        :hint="t('inviteExpiresDaysHint')"
      />
      <OaSelectField
        v-model="genForm.groupId"
        :label="t('inviteGroup')"
        :options="[
          { value: '', label: t('inviteGroupNone') },
          ...groups.map((group) => ({ value: group.id, label: group.name })),
        ]"
      />
      <template v-if="genForm.groupId">
        <OaNumberField v-model="genForm.daysFrom" :label="t('inviteGroupDaysFrom')" :min="0" :max="3650" :hint="t('inviteGroupDaysHint')" />
        <OaNumberField v-model="genForm.daysTo" :label="t('inviteGroupDaysTo')" :min="0" :max="3650" :placeholder="t('inviteGroupDaysFixed')" />
      </template>
      <OaTextField v-model="genForm.note" :label="t('inviteNote')" :hint="t('inviteNoteHint')" />
    </template>
  </OaPanel>
</template>
