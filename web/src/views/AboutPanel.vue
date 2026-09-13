<script setup lang="ts">
// The About panel.
//
// An instance can be renamed and can describe itself in Markdown however its
// operator wants. The facts below do not change with that copy: they identify
// the software a rebranded server is actually running and where it came from.

import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { health } from '@/api/client';
import OaMarkdown from '@/components/OaMarkdown.vue';
import OaPanel from '@/components/OaPanel.vue';
import { t } from '@/composables/useI18n';
import { formatUptime } from '@/lib/format';
import { siteInfo } from '@/stores/session';

const PRODUCT = 'Obsidian Arc';
const SOURCE_URL = 'https://github.com/OnyxAxisOwO/ObsidianArc';

const router = useRouter();

const version = ref('—');
const uptime = ref('—');

onMounted(() => {
  void health()
    .then((status) => {
      version.value = status.version;
      uptime.value = formatUptime(status.uptime_sec);
    })
    .catch(() => {
      // The facts box just keeps its placeholders; nothing else on the page
      // depends on the server being reachable.
    });
});
</script>

<template>
  <OaPanel
    :title="t('about')"
    :footer="false"
    :width="460"
    @close="router.replace('/')"
  >
    <div class="oa-about">
      <!-- Both fall back rather than render empty: an operator who has never
           opened the settings screen still gets a finished page. -->
      <h2 class="oa-about-name">{{ siteInfo.about?.title?.trim() || siteInfo.name }}</h2>

      <div class="oa-about-facts">
        <div class="oa-about-fact">
          <span class="oa-about-fact-label">{{ t('aboutVersionOf', { product: PRODUCT }) }}</span>
          <span class="oa-about-fact-value">{{ version }}</span>
        </div>

        <!-- Directly under the version, and showing the address rather than
             the word "Source": the two together are what identifies the
             software when everything above them has been rewritten. -->
        <div class="oa-about-fact">
          <span class="oa-about-fact-label">{{ t('aboutSource') }}</span>
          <a
            class="oa-about-fact-value oa-about-link"
            :href="SOURCE_URL"
            target="_blank"
            rel="noopener noreferrer"
          >{{ SOURCE_URL.replace('https://', '') }}</a>
        </div>

        <div class="oa-about-fact">
          <span class="oa-about-fact-label">{{ t('aboutRuntime') }}</span>
          <span class="oa-about-fact-value">{{ uptime }}</span>
        </div>
        <div class="oa-about-fact">
          <span class="oa-about-fact-label">{{ t('aboutLicense') }}</span>
          <span class="oa-about-fact-value">MIT</span>
        </div>
      </div>

      <section class="oa-about-details">
        <h3 class="oa-drawer-subhead">{{ t('aboutText') }}</h3>
        <!-- The transcript renderer builds DOM nodes from an allowlist rather
             than accepting HTML, so operator Markdown gets the same XSS
             boundary as model output. -->
        <OaMarkdown
          class="ai-answer oa-about-lede"
          :text="siteInfo.about?.body?.trim() || t('aboutBody')"
        />
      </section>
    </div>
  </OaPanel>
</template>
