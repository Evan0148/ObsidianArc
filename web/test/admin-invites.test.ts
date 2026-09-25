import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createApp, h, nextTick, shallowRef, type App, type Component } from 'vue';
import AdminInvites from '../src/views/admin/AdminInvites.vue';
import { adminApi, type InviteCode, type InviteStats, type InviteUse } from '../src/admin/api';
import * as authApi from '../src/api/auth';
import { provideAdminView } from '../src/views/admin/adminView';
import { providePanelHost } from '../src/composables/usePanelHost';
import { changeLanguage, t } from '../src/composables/useI18n';
import { siteInfo } from '../src/stores/session';

let app: App | undefined;
let host: HTMLElement;
let panels: HTMLElement;
let actions: HTMLElement;

beforeEach(async () => {
  await changeLanguage('en');
  host = document.createElement('div');
  panels = document.createElement('div');
  actions = document.createElement('div');
  document.body.append(host, panels, actions);
});

afterEach(() => {
  app?.unmount();
  app = undefined;
  document.body.textContent = '';
  vi.restoreAllMocks();
});

async function settle(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
  await nextTick();
}

async function mount(component: Component): Promise<void> {
  app = createApp({ setup() {
    providePanelHost(shallowRef(panels));
    provideAdminView({ actionsHost: actions, setTitle: () => {}, reload: () => {}, params: [] });
    return () => h(component);
  } });
  app.mount(host);
  await settle();
}

function button(root: ParentNode, label: string): HTMLButtonElement {
  const found = [...root.querySelectorAll<HTMLButtonElement>('button')].find((node) => node.textContent?.trim() === label);
  if (!found) throw new Error(`Button not found: ${label}`);
  return found;
}

function fieldFor(root: ParentNode, label: string): HTMLElement {
  const found = [...root.querySelectorAll<HTMLElement>('.oa-field')].find((field) =>
    field.querySelector('.oa-field-label')?.textContent === label);
  if (!found) throw new Error(`Field not found: ${label}`);
  return found;
}

/** `OaSwitchField` is a `.oa-checkbox-field` label, not an `.oa-field` — it
 *  carries no separate `.oa-field-label`, so the switches need their own
 *  lookup rather than sharing `fieldFor`. */
function switchFor(root: ParentNode, label: string): HTMLInputElement {
  const found = [...root.querySelectorAll<HTMLLabelElement>('.oa-checkbox-field')].find((field) =>
    field.querySelector('span')?.textContent === label);
  if (!found) throw new Error(`Switch not found: ${label}`);
  return found.querySelector<HTMLInputElement>('input[type="checkbox"]')!;
}

/** Opens an `OaSelect` and picks the option matching `optionLabel`. */
async function choose(trigger: HTMLButtonElement, optionLabel: string): Promise<void> {
  trigger.click();
  await settle();
  const option = [...document.querySelectorAll<HTMLElement>('[role="option"]')]
    .find((node) => node.textContent?.trim() === optionLabel);
  if (!option) throw new Error(`Option not found: ${optionLabel}`);
  option.click();
  await settle();
}

function input(node: HTMLInputElement, value: string): void {
  node.value = value;
  node.dispatchEvent(new Event('input', { bubbles: true }));
}

const noStats: InviteStats = { active: 0, uses_total: 0, uses_7d: 0, top_inviters: [] };

function stubBase(overrides: { settings?: Record<string, string> } = {}): void {
  vi.spyOn(adminApi, 'settings').mockResolvedValue({
    settings: {
      'registration.enabled': 'true',
      'invites.required': 'false',
      'invites.user_enabled': 'false',
      'invites.user_limit': '10',
      'invites.reward_cards': '0',
      'invites.reward_card_days': '30',
      ...overrides.settings,
    },
    groups: [],
  });
  vi.spyOn(adminApi, 'groupOptions').mockResolvedValue({ groups: [{ id: 'g1', name: 'Premium' }] });
  vi.spyOn(adminApi, 'inviteStats').mockResolvedValue(noStats);
  vi.spyOn(authApi, 'fetchSite').mockResolvedValue(siteInfo.value);
}

describe('registration mode and rewards', () => {
  it('derives the mode from the two settings it stands for, and writes them back together', async () => {
    stubBase({ settings: { 'registration.enabled': 'true', 'invites.required': 'true' } });
    vi.spyOn(adminApi, 'invites').mockResolvedValue({ codes: [], total: 0 });
    const save = vi.spyOn(adminApi, 'saveSettings').mockResolvedValue({ settings: {} });
    await mount(AdminInvites);

    const mode = fieldFor(host, t('registrationMode'));
    expect(mode.querySelector('.oa-select-label')?.textContent).toBe(t('registrationModeInvite'));

    await choose(mode.querySelector<HTMLButtonElement>('.oa-select')!, t('registrationModeClosed'));
    expect(actions.textContent).toContain(t('controlUnsaved'));

    button(actions, t('save')).click();
    await settle();

    expect(save).toHaveBeenCalledWith(expect.objectContaining({
      'registration.enabled': 'false',
      'invites.required': 'false',
    }));
    expect(actions.textContent).toContain(t('controlSaved'));
  });

  it('keeps the per-account limit and reward fields out of the form until personal invites are switched on', async () => {
    stubBase();
    vi.spyOn(adminApi, 'invites').mockResolvedValue({ codes: [], total: 0 });
    const save = vi.spyOn(adminApi, 'saveSettings').mockResolvedValue({ settings: {} });
    await mount(AdminInvites);

    expect(host.textContent).not.toContain(t('userInviteLimit'));

    const toggle = switchFor(host, t('userInvitesEnabled'));
    toggle.click();
    await settle();
    expect(host.textContent).toContain(t('userInviteLimit'));
    expect(host.textContent).toContain(t('inviteRewardCards'));

    button(actions, t('save')).click();
    await settle();
    expect(save).toHaveBeenCalledWith(expect.objectContaining({
      'invites.user_enabled': 'true',
      'invites.user_limit': '10',
    }));
  });
});

describe('the code list', () => {
  it('sends the kind, status and a debounced search to the server', async () => {
    stubBase();
    const list = vi.spyOn(adminApi, 'invites').mockResolvedValue({ codes: [], total: 0 });
    await mount(AdminInvites);
    expect(list).toHaveBeenLastCalledWith('?limit=20&offset=0');

    const [kind, status] = [...host.querySelectorAll<HTMLButtonElement>('.oa-filter-select')];
    await choose(kind!, t('inviteKindUser'));
    expect(list).toHaveBeenLastCalledWith('?limit=20&offset=0&kind=user');

    await choose(status!, t('inviteStatusRevoked'));
    expect(list).toHaveBeenLastCalledWith('?limit=20&offset=0&kind=user&status=revoked');

    input(host.querySelector<HTMLInputElement>('input[type="search"]')!, 'partner');
    await vi.waitFor(() => expect(list).toHaveBeenLastCalledWith('?limit=20&offset=0&kind=user&status=revoked&q=partner'));
  });
});

describe('generating codes', () => {
  it('only offers a name for a single code, and mints a batch otherwise', async () => {
    stubBase();
    vi.spyOn(adminApi, 'invites').mockResolvedValue({ codes: [], total: 0 });
    const create = vi.spyOn(adminApi, 'createInvites').mockImplementation(async (body) => ({
      codes: Array.from({ length: Number(body['count']) }, (_, index) => ({
        id: `new-${index}`, code: `CODE${index}0000`, owner_id: '', owner_username: '', owner_nickname: '',
        group_id: '', group_name: '', group_days: 0, group_days_max: 0,
        max_uses: Number(body['max_uses']), uses: 0, expires_at: 0, revoked_at: 0,
        note: String(body['note']), created_by: 'op', created_at: Date.now(), status: 'active',
      } satisfies InviteCode)),
    }));
    await mount(AdminInvites);

    button(actions, t('generateInvites')).click();
    await settle();
    expect(panels.textContent).toContain(t('inviteCustomCode'));

    const count = fieldFor(panels, t('inviteCount')).querySelector<HTMLInputElement>('input')!;
    count.value = '5';
    count.dispatchEvent(new Event('input', { bubbles: true }));
    await settle();
    expect(panels.textContent).not.toContain(t('inviteCustomCode'));

    button(panels, t('generateInvites')).click();
    await settle();

    expect(create).toHaveBeenCalledWith(expect.objectContaining({ count: 5, code: '' }));
    expect(panels.textContent).toContain(t('codesMinted', { count: 5 }));
  });

  // The result view replaces the form the moment a batch is minted — the
  // same panel AdminCodes uses for its own codes — so each shape of the
  // range gets its own generate, rather than a second submit from a form
  // that mounting the result view has already taken off screen.
  it('leaves an untouched range as a fixed, permanent length', async () => {
    stubBase();
    vi.spyOn(adminApi, 'invites').mockResolvedValue({ codes: [], total: 0 });
    const create = vi.spyOn(adminApi, 'createInvites').mockResolvedValue({ codes: [] });
    await mount(AdminInvites);

    button(actions, t('generateInvites')).click();
    await settle();
    await choose(fieldFor(panels, t('inviteGroup')).querySelector<HTMLButtonElement>('.oa-select')!, 'Premium');
    button(panels, t('generateInvites')).click();
    await settle();

    expect(create).toHaveBeenCalledWith(expect.objectContaining({
      group_id: 'g1', group_days: 0, group_days_max: 0,
    }));
  });

  it('sends a real range once "up to" is set higher than "days"', async () => {
    stubBase();
    vi.spyOn(adminApi, 'invites').mockResolvedValue({ codes: [], total: 0 });
    const create = vi.spyOn(adminApi, 'createInvites').mockResolvedValue({ codes: [] });
    await mount(AdminInvites);

    button(actions, t('generateInvites')).click();
    await settle();
    await choose(fieldFor(panels, t('inviteGroup')).querySelector<HTMLButtonElement>('.oa-select')!, 'Premium');
    input(fieldFor(panels, t('inviteGroupDaysFrom')).querySelector<HTMLInputElement>('input')!, '3');
    input(fieldFor(panels, t('inviteGroupDaysTo')).querySelector<HTMLInputElement>('input')!, '7');
    await settle();
    button(panels, t('generateInvites')).click();
    await settle();

    expect(create).toHaveBeenCalledWith(expect.objectContaining({
      group_id: 'g1', group_days: 3, group_days_max: 7,
    }));
  });
});

describe('an existing code', () => {
  const code: InviteCode = {
    id: 'inv1', code: 'ABCD1234', owner_id: '', owner_username: '', owner_nickname: '',
    group_id: 'g1', group_name: 'Premium', group_days: 3, group_days_max: 7,
    max_uses: 10, uses: 2, expires_at: 0, revoked_at: 0, note: 'Partner X',
    created_by: 'op', created_at: Date.now(), status: 'active',
  };
  const uses: InviteUse[] = [
    { user_id: 'u1', username: 'alice', nickname: 'Alice', group_days: 3, created_at: Date.now(), rewarded_at: Date.now(), reward_cards: 2, reward_skipped: '' },
    { user_id: 'u2', username: 'bob', nickname: '', group_days: 7, created_at: Date.now(), rewarded_at: 0, reward_cards: 0, reward_skipped: 'same_ip' },
  ];

  async function openRow(): Promise<void> {
    stubBase();
    vi.spyOn(adminApi, 'invites').mockResolvedValue({ codes: [code], total: 1 });
    vi.spyOn(adminApi, 'inviteUses').mockResolvedValue({ uses });
    await mount(AdminInvites);
    host.querySelector<HTMLTableRowElement>('tbody tr')!.click();
    await settle();
  }

  it('shows the range it grants, and who has used it so far', async () => {
    await openRow();
    expect(panels.textContent).toContain('ABCD-1234');
    expect(panels.textContent).toContain(t('invitesDaysRange', { from: 3, to: 7 }));
    expect(panels.textContent).toContain(t('inviteRewardedBadge'));
    expect(panels.textContent).toContain(t('inviteSkippedSameIP'));
  });

  it('revokes on the second press, and then offers nothing more to revoke', async () => {
    await openRow();
    const revoke = vi.spyOn(adminApi, 'revokeInvite').mockResolvedValue({ code: { ...code, status: 'revoked', revoked_at: Date.now() } });

    button(panels, t('revokeLabel')).click();
    await settle();
    expect(revoke).not.toHaveBeenCalled();

    button(panels, t('revokeLabel')).click();
    await settle();
    expect(revoke).toHaveBeenCalledWith('inv1');
    expect(() => button(panels, t('revokeLabel'))).toThrow();
  });
});
