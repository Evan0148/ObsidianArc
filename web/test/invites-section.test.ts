import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createApp, h, nextTick, shallowRef, type App, type Component } from 'vue';
import * as invitesApi from '../src/api/invites';
import type { ProfileInvites } from '../src/api/invites';
import { providePanelHost } from '../src/composables/usePanelHost';
import { changeLanguage, t } from '../src/composables/useI18n';
import InvitesSection from '../src/views/settings/InvitesSection.vue';

// The "Invites" settings card: an account's own personal code, its usage,
// its invitees, and regenerating it.

const ON: ProfileInvites = {
  enabled: true, code: 'ABCD2345', limit: 10, used: 1, reward_cards: 2, reward_card_days: 30,
  invitees: [
    { nickname: 'Ada', username: 'ada', created_at: Date.now(), rewarded: true, reward_skipped: '' },
  ],
};

let app: App | undefined;
let host: HTMLElement;

beforeEach(async () => {
  await changeLanguage('en');
  host = document.createElement('div');
  document.body.append(host);
});

afterEach(() => {
  app?.unmount();
  app = undefined;
  document.body.textContent = '';
  vi.restoreAllMocks();
});

async function settle(): Promise<void> {
  for (let i = 0; i < 3; i++) {
    await new Promise((resolve) => setTimeout(resolve, 0));
    await nextTick();
  }
}

async function mount(component: Component, props: Record<string, unknown> = {}): Promise<void> {
  const panels = document.createElement('div');
  document.body.append(panels);
  app = createApp({
    setup() {
      providePanelHost(shallowRef(panels));
      return () => h(component, props);
    },
  });
  app.mount(host);
  await settle();
}

function button(label: string): HTMLButtonElement {
  const found = [...host.querySelectorAll<HTMLButtonElement>('button')]
    .find((node) => node.textContent?.trim() === label);
  if (!found) throw new Error(`Button not found: ${label}`);
  return found;
}

describe('the invites card', () => {
  it('shows the personal code grouped as XXXX-XXXX, and stays hidden while off', async () => {
    vi.spyOn(invitesApi, 'fetchProfileInvites').mockResolvedValue({ ...ON, code: 'ABCD2345' });
    await mount(InvitesSection);

    expect(host.textContent).toContain('ABCD-2345');
  });

  it('draws nothing when the operator has personal codes turned off', async () => {
    vi.spyOn(invitesApi, 'fetchProfileInvites').mockResolvedValue({
      enabled: false, code: '', limit: 0, used: 0, reward_cards: 0, reward_card_days: 0, invitees: [],
    });
    await mount(InvitesSection);

    expect(host.textContent?.trim()).toBe('');
  });

  it('lists usage against the limit, and as unlimited when there is none', async () => {
    vi.spyOn(invitesApi, 'fetchProfileInvites').mockResolvedValue(ON);
    await mount(InvitesSection);
    expect(host.textContent).toContain(t('inviteUsageLimited', { used: 1, limit: 10 }));

    app?.unmount();
    host.textContent = '';
    vi.spyOn(invitesApi, 'fetchProfileInvites').mockResolvedValue({ ...ON, limit: 0 });
    await mount(InvitesSection);
    expect(host.textContent).toContain(t('inviteUsageUnlimited', { used: 1 }));
  });

  it('names who joined, and whether the invite was rewarded', async () => {
    vi.spyOn(invitesApi, 'fetchProfileInvites').mockResolvedValue({
      ...ON,
      invitees: [
        { nickname: 'Ada', username: 'ada', created_at: Date.now(), rewarded: true, reward_skipped: '' },
        { nickname: '', username: 'bob', created_at: Date.now(), rewarded: false, reward_skipped: 'same_ip' },
      ],
    });
    await mount(InvitesSection);

    expect(host.textContent).toContain('Ada');
    expect(host.textContent).toContain(t('inviteeRewarded'));
    // No nickname falls back to the username.
    expect(host.textContent).toContain('bob');
    expect(host.textContent).toContain(t('inviteeSkipSameIp'));
  });

  it('says nobody has joined yet, rather than an empty list', async () => {
    vi.spyOn(invitesApi, 'fetchProfileInvites').mockResolvedValue({ ...ON, invitees: [] });
    await mount(InvitesSection);
    expect(host.textContent).toContain(t('inviteesEmpty'));
  });

  it('regenerates the code behind a confirm, and shows the new one', async () => {
    vi.spyOn(invitesApi, 'fetchProfileInvites').mockResolvedValue(ON);
    const regenerate = vi.spyOn(invitesApi, 'regenerateProfileInvite')
      .mockResolvedValue({ ...ON, code: 'ZZZZ9999', used: 0, invitees: [] });
    await mount(InvitesSection);

    button(t('inviteRegenerate')).click();
    await settle();
    button(t('confirmWord')).click();
    await settle();

    expect(regenerate).toHaveBeenCalled();
    expect(host.textContent).toContain('ZZZZ-9999');
    expect(host.textContent).not.toContain('ABCD-2345');
  });
});
