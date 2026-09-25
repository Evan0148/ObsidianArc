import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { createApp, nextTick, type App as VueApp } from 'vue';
import { createMemoryHistory, createRouter } from 'vue-router';
import App from '../src/App.vue';
import { site, siteInfo } from '../src/stores/session';

// The browser tab used to say "Obsidian Arc" on every instance, because
// nothing ever wrote to document.title after the static <title> in
// index.html. App.vue now keeps it in sync with /api/site's browser_title —
// itself the operator's site.browser_title setting, or the site name when
// that is blank — and this is the one place that watcher lives, so it is the
// one place worth pinning.

let app: VueApp | undefined;
let host: HTMLElement;

beforeEach(() => {
  host = document.createElement('div');
  document.body.appendChild(host);
});

afterEach(() => {
  app?.unmount();
  app = undefined;
  host.remove();
  site.value = null;
});

async function mountApp(): Promise<void> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:pathMatch(.*)*', component: { render: () => null } }],
  });
  app = createApp(App);
  app.use(router);
  await router.isReady();
  app.mount(host);
  await nextTick();
}

describe('browser tab title', () => {
  it('uses the resolved browser_title from /api/site', async () => {
    site.value = { ...siteInfo.value, name: 'My Instance', browser_title: 'My Instance' };
    await mountApp();
    expect(document.title).toBe('My Instance');
  });

  it('prefers an explicit browser title over the site name', async () => {
    site.value = { ...siteInfo.value, name: 'My Instance', browser_title: 'Custom Tab Title' };
    await mountApp();
    expect(document.title).toBe('Custom Tab Title');
  });

  // An older server that has never heard of site.browser_title sends none at
  // all; the client falls back to the name itself rather than showing
  // nothing in the tab.
  it('falls back to the site name when the server sends no browser_title', async () => {
    const { browser_title: _dropped, ...withoutBrowserTitle } = { ...siteInfo.value, name: 'Older Server' };
    site.value = withoutBrowserTitle;
    await mountApp();
    expect(document.title).toBe('Older Server');
  });

  it('updates the open tab without a reload when the setting changes', async () => {
    site.value = { ...siteInfo.value, name: 'My Instance', browser_title: 'My Instance' };
    await mountApp();
    expect(document.title).toBe('My Instance');

    // What AdminSettings.vue's save() does to this same ref after a
    // successful PUT — this is the "no reload needed" half of the feature.
    site.value = { ...site.value!, browser_title: 'Renamed Tab' };
    await nextTick();
    expect(document.title).toBe('Renamed Tab');
  });
});
