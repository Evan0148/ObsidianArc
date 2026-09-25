<script setup lang="ts">
// The root. Everything is decided by the route, so there is nothing here but
// the outlet — and the one piece of global, always-on state that has nowhere
// else to live: the browser tab itself is not part of any route's template.

import { watchEffect } from 'vue';
import { siteInfo } from './stores/session';

// /api/site already resolves browser_title against the site's own name
// (settings.Service.BrowserTitle), so there is no second fallback to keep in
// sync here. Runs again whenever siteInfo changes — on the initial load, and
// after an operator saves the settings page while this tab is the one open —
// so the tab updates without a reload.
watchEffect(() => {
  document.title = siteInfo.value.browser_title || siteInfo.value.name;
});
</script>

<template>
  <RouterView />
</template>
