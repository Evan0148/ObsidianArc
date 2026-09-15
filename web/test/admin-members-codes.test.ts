import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createApp, h, nextTick, shallowRef, type App, type Component } from 'vue';
import AdminCodes from '../src/views/admin/AdminCodes.vue';
import GroupMembers from '../src/views/admin/GroupMembers.vue';
import { adminApi, type Group, type RedemptionCode } from '../src/admin/api';
import type { Account } from '../src/api/auth';
import { saveAsFile } from '../src/api/backup';
import { provideAdminView } from '../src/views/admin/adminView';
import { providePanelHost } from '../src/composables/usePanelHost';
import { changeLanguage, t } from '../src/composables/useI18n';

vi.mock('../src/api/backup', () => ({ saveAsFile: vi.fn() }));

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
  vi.mocked(saveAsFile).mockClear();
});

async function settle(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
  await nextTick();
}

async function mount(component: Component, props = {}): Promise<void> {
  app = createApp({ setup() {
    providePanelHost(shallowRef(panels));
    provideAdminView({ actionsHost: actions, setTitle: () => {}, reload: () => {}, params: [] });
    return () => h(component, props);
  } });
  app.mount(host);
  await settle();
}

function input(node: HTMLInputElement, value: string): void {
  node.value = value;
  node.dispatchEvent(new Event('input', { bubbles: true }));
}

function button(root: ParentNode, label: string): HTMLButtonElement {
  const found = [...root.querySelectorAll<HTMLButtonElement>('button')].find((node) => node.textContent?.trim() === label);
  if (!found) throw new Error(`Button not found: ${label}`);
  return found;
}

function code(value: string, at: string): RedemptionCode {
  return { id: value, code: value, cards: 1, claimed: 0, card_days: 10, expires_at: 0, note: '', created_at: new Date(at).getTime() };
}

describe('redemption code export', () => {
  const records = [
    code('BEFORE', '2026-09-13T11:59:59'),
    code('START', '2026-09-13T12:00:00'),
    { ...code('END', '2026-09-13T12:30:59.999'), claimed: 1, expires_at: 1 },
    code('AFTER', '2026-09-13T12:31:00'),
  ];

  it('exports creation-time matches, including the whole last minute and redeemed codes', async () => {
    vi.spyOn(adminApi, 'codes').mockResolvedValue({ codes: records });
    await mount(AdminCodes);
    actions.querySelector<HTMLButtonElement>('#exportCodes')!.click();
    await settle();
    const fields = panels.querySelectorAll<HTMLInputElement>('input[type="datetime-local"]');
    input(fields[0]!, '2026-09-13T12:00');
    input(fields[1]!, '2026-09-13T12:30');
    await settle();
    expect(panels.textContent).toContain(t('codeExportCount', { count: 2 }));
    button(panels, t('download')).click();
    expect(saveAsFile).toHaveBeenCalledWith(expect.stringMatching(/\.txt$/), 'START\nEND\n', 'text/plain;charset=utf-8');
  });

  it('supports open bounds and refuses reversed or empty ranges', async () => {
    vi.spyOn(adminApi, 'codes').mockResolvedValue({ codes: records });
    await mount(AdminCodes);
    actions.querySelector<HTMLButtonElement>('#exportCodes')!.click();
    await settle();
    expect(panels.textContent).toContain(t('codeExportCount', { count: 4 }));
    const fields = panels.querySelectorAll<HTMLInputElement>('input[type="datetime-local"]');
    input(fields[0]!, '2026-09-13T12:00');
    await settle();
    expect(panels.textContent).toContain(t('codeExportCount', { count: 3 }));
    input(fields[1]!, '2026-09-13T11:00');
    await settle();
    expect(panels.textContent).toContain(t('codeExportInvalidRange'));
    expect(panels.querySelector('.oa-panel-foot .primary')).toBeNull();
    input(fields[0]!, '');
    await settle();
    expect(panels.textContent).toContain(t('codeExportCount', { count: 0 }));
    expect(panels.querySelector('.oa-panel-foot .primary')).toBeNull();
    expect(saveAsFile).not.toHaveBeenCalled();
  });
});

const group = { id: 'premium', name: 'Premium' } as Group;
const accounts = Array.from({ length: 25 }, (_, index) => ({
  id: `user-${index}`, username: `member${index}`, nickname: '', group_id: group.id,
  group_expires_at: 0,
} as Account));

describe('group member selection', () => {
  function users(): ReturnType<typeof vi.spyOn> {
    return vi.spyOn(adminApi, 'users').mockImplementation(async (query) => {
      const params = new URLSearchParams(query);
      const offset = Number(params.get('offset'));
      return { users: accounts.slice(offset, offset + 20), total: accounts.length };
    });
  }

  it('keeps selections across pages and submits one membership update with a local expiry', async () => {
    const listing = users();
    const assign = vi.spyOn(adminApi, 'assignGroupMembers').mockResolvedValue({ updated: 2 });
    await mount(GroupMembers, { group, groups: [group] });
    expect(listing).toHaveBeenCalledWith(expect.stringContaining('group_id=premium'));
    host.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click();
    button(host, t('next')).click();
    await settle();
    host.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click();
    const expiry = '2099-10-11T12:30';
    input(host.querySelector<HTMLInputElement>('input[type="datetime-local"]')!, expiry);
    await settle();
    expect(host.textContent).toContain(t('membersSelected', { count: 2 }));
    button(host, t('saveMembers')).click();
    await settle();
    expect(assign).toHaveBeenCalledWith('premium', ['user-0', 'user-20'], new Date(expiry).getTime());
    expect(host.textContent).toContain(t('membersSelected', { count: 0 }));
    expect(host.textContent).toContain(t('membersSaved', { count: 2 }));
  });

  it('searches all accounts on the server, rejects past expiry and preserves a failed selection', async () => {
    const listing = users();
    const assign = vi.spyOn(adminApi, 'assignGroupMembers').mockRejectedValue(new Error('Save failed'));
    await mount(GroupMembers, { group, groups: [group] });
    host.querySelector<HTMLButtonElement>('.oa-select')!.click();
    await settle();
    [...document.querySelectorAll<HTMLElement>('[role="option"]')].find((node) => node.textContent?.trim() === t('allAccounts'))!.click();
    input(host.querySelector<HTMLInputElement>('input[type="search"]')!, 'far-away-account');
    await vi.waitFor(() => expect(listing).toHaveBeenLastCalledWith('?q=far-away-account&limit=20&offset=0'));
    await settle();
    host.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click();
    input(host.querySelector<HTMLInputElement>('input[type="datetime-local"]')!, '2001-01-01T00:00');
    await settle();
    button(host, t('saveMembers')).click();
    await settle();
    expect(assign).not.toHaveBeenCalled();
    expect(host.textContent).toContain(t('membershipExpiryInvalid'));
    input(host.querySelector<HTMLInputElement>('input[type="datetime-local"]')!, '');
    await settle();
    button(host, t('saveMembers')).click();
    await settle();
    expect(assign).toHaveBeenCalledWith('premium', ['user-0'], 0);
    expect(host.textContent).toContain('Save failed');
    expect(host.querySelector<HTMLInputElement>('input[type="checkbox"]')!.checked).toBe(true);
  });
});
