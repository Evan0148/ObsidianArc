<script setup lang="ts">
// Everything between the front door and an account.
//
// Split out of the settings screen because these are one subject and it was
// not: whether registration is open, what a new account must supply, how fast
// addresses may open them, whether a browser has to prove itself, and whether
// a model is asked to look at the result. An operator dealing with a wave of
// junk accounts opens one page, not seven sections of another.

import { computed, onMounted, ref } from 'vue';
import { adminApi, type AdminModel, type Group, type SecurityEvent, type SignInApplication } from '@/admin/api';
import { fetchSite } from '@/api/auth';
import { ApiError } from '@/api/client';
import OaPagination from '@/components/OaPagination.vue';
import type { PageState } from '@/components/table-types';
import OaBadge from '@/components/OaBadge.vue';
import OaConfirmButton from '@/components/OaConfirmButton.vue';
import AdminControlCard from './AdminControlCard.vue';
import AdminWorkbench from './AdminWorkbench.vue';
import type { WorkbenchGroup } from './workbench';
import { useSettingsDraft } from './settingsDraft';
import { IconUsers, IconLock, IconSpark, IconFile, IconSliders, IconKey, IconGithub, IconGoogle, IconCopy, IconCheck } from '@/icons';
import OaNumberField from '@/components/OaNumberField.vue';
import OaSelectField from '@/components/OaSelectField.vue';
import OaSwitchField from '@/components/OaSwitchField.vue';
import OaTextArea from '@/components/OaTextArea.vue';
import OaTextField from '@/components/OaTextField.vue';
import { t } from '@/composables/useI18n';
import { copyToClipboard } from '@/chat/markdown';
import { initials } from '@/lib/account';
import { absoluteTime } from '@/lib/format';
import { site } from '@/stores/session';
import AdminFailure from './AdminFailure.vue';
import { useAdminView } from './adminView';

const view = useAdminView();
view.setTitle(t('navSecurity'), t('securitySubtitle'));

const error = ref('');
const loaded = ref(false);
const mailConfigured = ref(false);
const groups = ref<Pick<Group, 'id' | 'name'>[]>([]);
const models = ref<Pick<AdminModel, 'id' | 'display_name' | 'model_id' | 'enabled' | 'provider_name'>[]>([]);
const flash = ref('');
const saveLabel = ref('');
const busy = ref(false);
const events = ref<SecurityEvent[]>([]);
const eventsTotal = ref(0);
const eventPage = ref<PageState>({ page: 1, pageSize: 20 });
let eventRequest = 0;
function changeEvents(next: PageState): void { eventPage.value = next; void loadEvents(); }
const eventsLoading = ref(false);

const form = ref({
  registration: false,
  defaultGroup: '',
  requireEmail: false,
  verifyEmail: false,
  emailDomains: '',
  qqRequirement: 'off',
  perMinute: 0 as number | null,
  perHour: 0 as number | null,
  perIP: 0 as number | null,
  ipWindow: 60 as number | null,
  turnstileSiteKey: '',
  turnstileSecret: '',
  turnstileSecretHint: '',
  turnstileOnLogin: false,
  turnstileOnSignup: false,
  turnstileOnAPIKey: false,
  turnstileOnRedeem: false,
  turnstileOnFeedback: false,
  chatChallengeRequests: 0 as number | null,
  chatChallengeWindowSecs: 60 as number | null,
  chatChallengeClearMins: 30 as number | null,
  reviewEnabled: false,
  reviewModel: '',
  reviewMode: 'normal',
  reviewRestrictHours: 24 as number | null,
  reviewRefusal: '',
  githubEnabled: false,
  githubClientID: '',
  githubSecret: '',
  githubSecretHint: '',
  googleEnabled: false,
  googleClientID: '',
  googleSecret: '',
  googleSecretHint: '',
  oauthAllowSignup: true,
  oauthLinkByEmail: true,
  oauthRequireQQ: false,
});



// --- the applications that may sign people in with an account here -------------
//
// Not a settings card: these are rows rather than fields, and they commit on
// their own buttons. They live on this screen anyway because they are the
// same subject as everything else on it — who gets in, and how.

const applications = ref<SignInApplication[]>([]);
const issuer = ref('');
const appFlash = ref('');
const appBusy = ref(false);
/** The client secret, for the one moment it exists. */
const freshSecret = ref('');
const draft = ref({ name: '', description: '', redirects: '', public: false, trusted: false });
/** The value that was last copied, so the control can say it landed. */
const copied = ref('');

function copy(value: string): void {
  void copyToClipboard(value).then((ok) => {
    if (!ok) return;
    copied.value = value;
    window.setTimeout(() => {
      if (copied.value === value) copied.value = '';
    }, 1500);
  });
}

/**
 * What to paste into the provider's own console.
 *
 * Built from the server's own idea of this instance's address, not from the
 * address bar. The two can differ — a proxy chain that loses the original
 * scheme leaves the server believing it is http where the browser knows it is
 * https — and when they differ it is the server's value that the provider is
 * shown at the exchange. A callback that differs from the registered one by a
 * scheme is refused with an error that names neither, so the screen has to
 * show the one that will actually be sent.
 *
 * The address bar is the fallback for the moment before the applications
 * list has answered.
 */
function callbackURL(provider: string): string {
  return `${issuer.value || window.location.origin}/api/auth/oauth/callback/${provider}`;
}

async function loadApplications(): Promise<void> {
  try {
    const result = await adminApi.applications();
    applications.value = result.applications ?? [];
    issuer.value = result.issuer;
  } catch (failure) {
    appFlash.value = failure instanceof ApiError ? failure.message : String(failure);
  }
}

function registerApplication(): void {
  if (appBusy.value) return;
  appBusy.value = true;
  appFlash.value = '';
  freshSecret.value = '';
  void adminApi.createApplication({
    name: draft.value.name.trim(),
    description: draft.value.description.trim(),
    redirect_uris: draft.value.redirects,
    public: draft.value.public,
    trusted: draft.value.trusted,
  })
    .then((result) => {
      freshSecret.value = result.client_secret;
      draft.value = { name: '', description: '', redirects: '', public: false, trusted: false };
      return loadApplications();
    })
    .catch((failure: unknown) => {
      appFlash.value = failure instanceof ApiError ? failure.message : String(failure);
    })
    .finally(() => { appBusy.value = false; });
}

function toggleApplication(app: SignInApplication, disabled: boolean): void {
  appBusy.value = true;
  appFlash.value = '';
  void adminApi.updateApplication(app.id, { disabled })
    .then(loadApplications)
    .catch((failure: unknown) => {
      appFlash.value = failure instanceof ApiError ? failure.message : String(failure);
    })
    .finally(() => { appBusy.value = false; });
}

function rotateApplication(app: SignInApplication): void {
  appBusy.value = true;
  appFlash.value = '';
  freshSecret.value = '';
  void adminApi.rotateApplicationSecret(app.id)
    .then((result) => { freshSecret.value = result.client_secret; })
    .catch((failure: unknown) => {
      appFlash.value = failure instanceof ApiError ? failure.message : String(failure);
    })
    .finally(() => { appBusy.value = false; });
}

function removeApplication(app: SignInApplication): void {
  appBusy.value = true;
  appFlash.value = '';
  void adminApi.deleteApplication(app.id)
    .then(loadApplications)
    .catch((failure: unknown) => {
      appFlash.value = failure instanceof ApiError ? failure.message : String(failure);
    })
    .finally(() => { appBusy.value = false; });
}

// Trying the reviewer on an account that is not being created.
const trial = ref({ username: '', email: '', qq: '', answer: '', running: false });

const enabledModels = computed(() => models.value.filter((entry) => entry.enabled));

/**
 * Only this page's keys. The settings endpoint writes what it is given and
 * leaves the rest alone, which is what lets two screens edit one store
 * without either one reverting the other's fields.
 */
function collect(): Record<string, string> {
  return {
    'registration.enabled': String(form.value.registration),
    'registration.default_group': form.value.defaultGroup,
    'registration.require_email': String(form.value.requireEmail),
    'registration.verify_email': String(form.value.verifyEmail),
    'registration.email_domains': form.value.emailDomains.trim(),
    'registration.qq_requirement': form.value.qqRequirement,
    'registration.per_minute': String(form.value.perMinute ?? 0),
    'registration.per_hour': String(form.value.perHour ?? 0),
    'registration.per_ip': String(form.value.perIP ?? 0),
    'registration.per_ip_window_minutes': String(form.value.ipWindow ?? 60),
    'turnstile.site_key': form.value.turnstileSiteKey.trim(),
    // Empty keeps what is stored: the field was never shown the secret, so
    // sending its emptiness back would erase it.
    'turnstile.secret_key': form.value.turnstileSecret.trim(),
    'turnstile.on_login': String(form.value.turnstileOnLogin),
    'turnstile.on_signup': String(form.value.turnstileOnSignup),
    'turnstile.on_api_key': String(form.value.turnstileOnAPIKey),
    'turnstile.on_redeem': String(form.value.turnstileOnRedeem),
    'turnstile.on_feedback': String(form.value.turnstileOnFeedback),
    'security.chat_challenge_requests': String(form.value.chatChallengeRequests ?? 0),
    'security.chat_challenge_window_seconds': String(form.value.chatChallengeWindowSecs ?? 60),
    'security.chat_challenge_clear_minutes': String(form.value.chatChallengeClearMins ?? 30),
    'security.signup_review': String(form.value.reviewEnabled),
    'security.signup_review_model': form.value.reviewModel,
    'security.signup_review_mode': form.value.reviewMode,
    'security.signup_review_restrict_hours': String(form.value.reviewRestrictHours ?? 24),
    'security.signup_review_refusal': form.value.reviewRefusal.trim(),
    'oauth.github_enabled': String(form.value.githubEnabled),
    'oauth.github_client_id': form.value.githubClientID.trim(),
    // Empty keeps what is stored, the same bargain the Turnstile secret
    // makes: the field was never shown the secret, so sending its emptiness
    // back would erase it.
    'oauth.github_client_secret': form.value.githubSecret.trim(),
    'oauth.google_enabled': String(form.value.googleEnabled),
    'oauth.google_client_id': form.value.googleClientID.trim(),
    'oauth.google_client_secret': form.value.googleSecret.trim(),
    'oauth.allow_signup': String(form.value.oauthAllowSignup),
    'oauth.link_by_email': String(form.value.oauthLinkByEmail),
    'oauth.require_qq': String(form.value.oauthRequireQQ),
  };
}

const { dirty, accept } = useSettingsDraft(collect);

async function save(): Promise<void> {
  if (!loaded.value || error.value || busy.value) return;
  const values = collect();
  busy.value = true;
  saveLabel.value = t('saving');
  flash.value = '';
  try {
    await adminApi.saveSettings(values);
    accept(values);
    try {
      site.value = await fetchSite();
    } catch {
      // The setting is already saved. A failed public-settings refresh should
      // not report that write as failed; the next page load will fetch it.
    }
    saveLabel.value = t('saved');
    window.setTimeout(() => { saveLabel.value = ''; }, 1500);
  } catch (failure) {
    flash.value = failure instanceof ApiError ? failure.message : String(failure);
    saveLabel.value = '';
  } finally {
    busy.value = false;
  }
}

/**
 * It exists because "the review is not working" and "the review is working
 * and being generous" look identical from outside: both are a registration
 * that went through. Type the account that got past it and read what the
 * model actually said — including that it could not be reached, which is the
 * state in which everything is allowed.
 */
function runTrial(): void {
  trial.value.running = true;
  trial.value.answer = t('reviewTrying');
  void adminApi.tryReview({
    username: trial.value.username.trim(),
    email: trial.value.email.trim(),
    qq: trial.value.qq.trim(),
    // What a browser would have sent, so the answer is about the details and
    // not about a missing user agent.
    user_agent: navigator.userAgent,
  })
    .then((result) => {
      trial.value.answer = !result.ran
        ? t('reviewTryBroken', { decision: reviewDecision(result.decision), reason: result.reason })
        : t(result.decision === 'allow'
          ? 'reviewTryAllowed'
          : result.decision === 'restrict' ? 'reviewTryRestricted' : 'reviewTryRefused',
        { reason: result.reason });
    })
    .catch((failure: unknown) => {
      trial.value.answer = failure instanceof ApiError ? failure.message : String(failure);
    })
    .finally(() => { trial.value.running = false; });
}

function reviewDecision(decision: string): string {
  if (decision === 'allow') return t('securityDecisionAllow');
  if (decision === 'refuse') return t('securityDecisionRefuse');
  if (decision === 'required') return t('securityDecisionRequired');
  if (decision === 'passed') return t('securityDecisionPassed');
  if (decision === 'failed') return t('securityDecisionFailed');
  return t('securityDecisionRestrict');
}

function eventLabel(event: string): string {
  if (event === 'signup_review') return t('securityEventSignupReview');
  if (event === 'api_restriction') return t('securityEventAPIRestriction');
  if (event === 'api_restriction_lifted') return t('securityEventAPIRestrictionLifted');
  if (event === 'chat_challenge') return t('securityEventChatChallenge');
  return event;
}

async function loadEvents(): Promise<void> {
  const ticket = ++eventRequest;
  eventsLoading.value = true;
  try {
    const result = await adminApi.securityEvents(`?limit=${eventPage.value.pageSize}&offset=${(eventPage.value.page - 1) * eventPage.value.pageSize}`);
    if (ticket !== eventRequest) return;
    events.value = result.events ?? [];
    eventsTotal.value = result.total;
  } catch (failure) {
    flash.value = failure instanceof ApiError ? failure.message : String(failure);
  } finally {
    if (ticket === eventRequest) eventsLoading.value = false;
  }
}

async function load(): Promise<void> {
  error.value = '';
  try {
    // The models come along because one of these settings is which model
    // reviews a sign-up, and a select needs its options. The groups arrive
    // with the settings already.
    const [data, modelsResult] = await Promise.all([
      adminApi.settings(), adminApi.modelOptions(), loadEvents(), loadApplications(),
    ]);
    const values = data.settings;
    mailConfigured.value = data.mail_configured ?? false;
    groups.value = data.groups ?? [];
    models.value = modelsResult.models;


    form.value = {
      registration: values['registration.enabled'] === 'true',
      defaultGroup: values['registration.default_group'] ?? '',
      requireEmail: values['registration.require_email'] === 'true',
      verifyEmail: values['registration.verify_email'] === 'true',
      emailDomains: values['registration.email_domains'] ?? '',
      qqRequirement: values['registration.qq_requirement'] ?? 'off',
      perMinute: Number(values['registration.per_minute'] ?? 0),
      perHour: Number(values['registration.per_hour'] ?? 0),
      perIP: Number(values['registration.per_ip'] ?? 0),
      ipWindow: Number(values['registration.per_ip_window_minutes'] ?? 60),
      turnstileSiteKey: values['turnstile.site_key'] ?? '',
      turnstileSecret: '',
      turnstileSecretHint: values['turnstile.secret_key'] ?? '',
      turnstileOnLogin: values['turnstile.on_login'] === 'true',
      turnstileOnSignup: values['turnstile.on_signup'] === 'true',
      turnstileOnAPIKey: values['turnstile.on_api_key'] === 'true',
      turnstileOnRedeem: values['turnstile.on_redeem'] === 'true',
      turnstileOnFeedback: values['turnstile.on_feedback'] === 'true',
      chatChallengeRequests: Number(values['security.chat_challenge_requests'] ?? 0),
      chatChallengeWindowSecs: Number(values['security.chat_challenge_window_seconds'] ?? 60),
      chatChallengeClearMins: Number(values['security.chat_challenge_clear_minutes'] ?? 30),
      reviewEnabled: values['security.signup_review'] === 'true',
      reviewModel: values['security.signup_review_model'] ?? '',
      reviewMode: values['security.signup_review_mode'] ?? 'normal',
      reviewRestrictHours: Number(values['security.signup_review_restrict_hours'] ?? 24),
      reviewRefusal: values['security.signup_review_refusal'] ?? '',
      githubEnabled: values['oauth.github_enabled'] === 'true',
      githubClientID: values['oauth.github_client_id'] ?? '',
      githubSecret: '',
      githubSecretHint: values['oauth.github_client_secret'] ?? '',
      googleEnabled: values['oauth.google_enabled'] === 'true',
      googleClientID: values['oauth.google_client_id'] ?? '',
      googleSecret: '',
      googleSecretHint: values['oauth.google_client_secret'] ?? '',
      oauthAllowSignup: (values['oauth.allow_signup'] ?? 'true') === 'true',
      oauthLinkByEmail: (values['oauth.link_by_email'] ?? 'true') === 'true',
      oauthRequireQQ: values['oauth.require_qq'] === 'true',
    };
    accept();
  } catch (failure) {
    error.value = failure instanceof Error ? failure.message : String(failure);
  } finally {
    loaded.value = true;
  }
}

const categories: WorkbenchGroup[] = [
  { id: 'accounts', label: 'controlAccounts', hint: 'controlAccountsHint', icon: IconUsers, sections: ['secAccounts', 'secRegistration', 'secRegistrationLimits'] },
  { id: 'verification', label: 'controlVerification', hint: 'controlVerificationHint', icon: IconLock, sections: ['secTurnstile', 'secVerificationScenes', 'secChatChallenge'] },
  { id: 'review', label: 'controlReview', hint: 'controlReviewHint', icon: IconSpark, sections: ['secSignupReview', 'secReviewTrial'] },
  { id: 'signin', label: 'controlSignIn', hint: 'controlSignInHint', icon: IconGithub, sections: ['secOAuth', 'secApplications'] },
  { id: 'events', label: 'controlEvents', hint: 'controlEventsHint', icon: IconFile, sections: ['secSecurityLog'] },
];

const columns: [string[], string[]] = [['secAccounts', 'secRegistrationLimits', 'secTurnstile', 'secChatChallenge', 'secSignupReview'], ['secRegistration', 'secVerificationScenes', 'secReviewTrial']];

onMounted(load);
</script>

<template>
  <Teleport :to="view.actionsHost">
    <span v-if="loaded && !error" class="oa-control-save-state" :class="{ dirty }" role="status">
      <span class="oa-dashboard-dot" />{{ dirty ? t('controlUnsaved') : t('controlSaved') }}
    </span>
    <button type="button" class="oa-btn primary" :disabled="busy || !loaded || !!error" @click="save">
      {{ saveLabel || t('save') }}
    </button>
  </Teleport>
  <AdminFailure v-if="error" :message="error" @retry="load" />
  <p v-else-if="!loaded" class="oa-table-empty">{{ t('loading') }}</p>
  <AdminWorkbench v-else page="security" :groups="categories" :columns="columns">
    <template #left="{ visible }">
      <AdminControlCard id="secAccounts" v-show="visible('secAccounts')" :title="t('secAccounts')" :icon="IconUsers">
        <OaSwitchField
          v-model="form.registration"
          :label="t('anyoneCanRegister')"
          :hint="t('anyoneCanRegisterHint')"
        />
        <OaSelectField
          v-model="form.defaultGroup"
          :label="t('newAccountsJoin')"
          :options="[
            { value: '', label: t('theDefaultGroup') },
            ...groups.map((group) => ({ value: group.id, label: group.name })),
          ]"
        />
      </AdminControlCard>
      <AdminControlCard id="secRegistrationLimits" v-show="visible('secRegistrationLimits')" :title="t('controlRegistrationLimits')" :icon="IconSliders" :hint="t('controlRegistrationLimitsHint')">
        <OaNumberField v-model="form.perMinute" :label="t('signupsPerMinute')" :min="0" :max="1000" />
        <OaNumberField
          v-model="form.perHour"
          :label="t('signupsPerHour')"
          :min="0"
          :max="10000"
          :hint="t('signupThrottleHint')"
        />
        <OaNumberField v-model="form.perIP" :label="t('signupsPerIP')" :min="0" :hint="t('signupsPerIPHint')" />
        <OaNumberField
          v-model="form.ipWindow"
          :label="t('signupsIPWindow')"
          :min="1"
          :hint="t('signupsIPWindowHint')"
        />
      </AdminControlCard>
      <AdminControlCard id="secTurnstile" v-show="visible('secTurnstile')" :title="t('secTurnstile')" :icon="IconKey" :hint="t('turnstileHint')">
        <OaTextField
          v-model="form.turnstileSiteKey"
          :label="t('turnstileSiteKey')"
          placeholder="0x4AAAAAAA…"
          :hint="t('turnstileSiteKeyHint')"
          monospace
        />
        <OaTextField
          v-model="form.turnstileSecret"
          type="password"
          :label="t('turnstileSecretKey')"
          :placeholder="form.turnstileSecretHint || '0x4AAAAAAA…'"
          :hint="t('turnstileSecretHint')"
          monospace
        />
      </AdminControlCard>
      <AdminControlCard id="secChatChallenge" v-show="visible('secChatChallenge')" :title="t('controlChatChallenge')" :icon="IconSpark" :hint="t('controlChatChallengeHint')">
        <OaNumberField
          v-model="form.chatChallengeRequests"
          :label="t('chatChallengeRequests')"
          :hint="t('chatChallengeRequestsHint')"
          :min="0"
          :max="1000"
        />
        <template v-if="(form.chatChallengeRequests ?? 0) > 0">
          <OaNumberField
            v-model="form.chatChallengeWindowSecs"
            :label="t('chatChallengeWindow')"
            :hint="t('chatChallengeWindowHint')"
            :min="5"
            :max="3600"
          />
          <OaNumberField
            v-model="form.chatChallengeClearMins"
            :label="t('chatChallengeClearance')"
            :hint="t('chatChallengeClearanceHint')"
            :min="1"
            :max="1440"
          />
        </template>
      </AdminControlCard>
      <AdminControlCard id="secSignupReview" v-show="visible('secSignupReview')" :title="t('secSignupReview')" :icon="IconSpark" :hint="t('signupReviewIntro')">
        <OaSwitchField v-model="form.reviewEnabled" :label="t('signupReview')" :hint="t('signupReviewHint')" />
        <OaSelectField
          v-model="form.reviewModel"
          :label="t('signupReviewModel')"
          :hint="t('signupReviewModelHint')"
          :options="[
            { value: '', label: t('signupReviewNoModel') },
            ...enabledModels.map((entry) => ({ value: entry.id, label: entry.display_name })),
          ]"
        />
        <OaSelectField
          v-model="form.reviewMode"
          :label="t('signupReviewMode')"
          :hint="t('signupReviewModeHint')"
          :options="[
            { value: 'loose', label: t('reviewModeLoose') },
            { value: 'normal', label: t('reviewModeNormal') },
            { value: 'strict', label: t('reviewModeStrict') },
          ]"
        />
        <OaNumberField
          v-model="form.reviewRestrictHours"
          :label="t('signupReviewRestrictHours')"
          :hint="t('signupReviewRestrictHoursHint')"
          :min="0"
          :max="8760"
        />
        <OaTextArea
          v-model="form.reviewRefusal"
          :label="t('signupReviewRefusal')"
          :rows="3"
          :placeholder="t('signupReviewRefusalPlaceholder')"
          :hint="t('signupReviewRefusalHint')"
        />
      </AdminControlCard>
    </template>
    <template #right="{ visible }">
      <AdminControlCard id="secRegistration" v-show="visible('secRegistration')" :title="t('secRegistration')" :icon="IconUsers">
        <OaSwitchField v-model="form.requireEmail" :label="t('requireEmail')" :hint="t('requireEmailHint')" />
        <!-- Offered but inert without SMTP, and the hint says so. The server
             ignores it in that state too, so an operator cannot lock every new
             account out of an instance that cannot send the link. -->
        <div :class="{ 'oa-field-inert': !mailConfigured }">
          <OaSwitchField
            v-model="form.verifyEmail"
            :label="t('verifyEmail')"
            :hint="mailConfigured ? t('verifyEmailHint') : t('verifyEmailNoMail')"
          />
        </div>
        <OaTextArea
          v-model="form.emailDomains"
          :label="t('emailDomains')"
          :rows="2"
          :placeholder="t('emailDomainsPlaceholder')"
          :hint="t('emailDomainsHint')"
        />
        <OaSelectField
          v-model="form.qqRequirement"
          :label="t('qqRequirement')"
          :hint="t('qqRequirementHint')"
          :options="[
            { value: 'off', label: t('qqRequirementOff') },
            { value: 'optional', label: t('qqRequirementOptional') },
            { value: 'required', label: t('qqRequirementRequired') },
          ]"
        />
      </AdminControlCard>
      <AdminControlCard id="secVerificationScenes" v-show="visible('secVerificationScenes')" :title="t('controlVerificationScenes')" :icon="IconLock" :hint="t('controlVerificationScenesHint')">
        <OaSwitchField
          v-model="form.turnstileOnLogin"
          :label="t('turnstileOnLogin')"
          :hint="t('turnstileOnLoginHint')"
        />
        <OaSwitchField
          v-model="form.turnstileOnSignup"
          :label="t('turnstileOnSignup')"
          :hint="t('turnstileOnSignupHint')"
        />
        <OaSwitchField
          v-model="form.turnstileOnAPIKey"
          :label="t('turnstileOnAPIKey')"
          :hint="t('turnstileOnAPIKeyHint')"
        />
        <OaSwitchField
          v-model="form.turnstileOnRedeem"
          :label="t('turnstileOnRedeem')"
          :hint="t('turnstileOnRedeemHint')"
        />
        <OaSwitchField
          v-model="form.turnstileOnFeedback"
          :label="t('turnstileOnFeedback')"
          :hint="t('turnstileOnFeedbackHint')"
        />
      </AdminControlCard>
      <AdminControlCard id="secReviewTrial" v-show="visible('secReviewTrial')" :title="t('reviewTry')" :icon="IconSliders" :hint="t('reviewTryHint')">
        <div class="oa-field">
          <OaTextField v-model="trial.username" :label="t('username')" placeholder="123123123123" />
          <OaTextField v-model="trial.email" :label="t('email')" placeholder="123123123123@qq.com" />
          <OaTextField v-model="trial.qq" :label="t('qq')" placeholder="123123123123" />
          <button type="button" class="oa-btn" :disabled="trial.running" @click="runTrial">
            {{ t('reviewTryRun') }}
          </button>
          <p class="oa-field-hint">{{ trial.answer }}</p>
        </div>
      </AdminControlCard>
    </template>
    <template #default="{ visible }">
      <!-- Both halves of the sign-in tab are full-width rows: one narrow card
           beside one wide one reads as a mistake, and these two are the same
           kind of thing pointing in opposite directions. -->
      <AdminControlCard id="secOAuth" v-show="visible('secOAuth')" :title="t('secOAuth')" :icon="IconGithub" :hint="t('oauthHint')" class="oa-control-card-wide">
        <div class="oa-providers">
          <div class="oa-provider">
            <span class="oa-provider-mark"><IconGithub :size="15" /></span>
            <OaSwitchField v-model="form.githubEnabled" :label="t('oauthGitHub')" :hint="t('oauthGitHubHint')" />
            <OaTextField
              v-model="form.githubClientID"
              :label="t('oauthClientID')"
              placeholder="Iv1.…"
              monospace
            />
            <OaTextField
              v-model="form.githubSecret"
              type="password"
              :label="t('oauthClientSecret')"
              :placeholder="form.githubSecretHint || '••••'"
              :hint="t('oauthCallback', { url: callbackURL('github') })"
              monospace
            />
          </div>
          <div class="oa-provider">
            <span class="oa-provider-mark"><IconGoogle :size="15" /></span>
            <OaSwitchField v-model="form.googleEnabled" :label="t('oauthGoogle')" :hint="t('oauthGoogleHint')" />
            <OaTextField
              v-model="form.googleClientID"
              :label="t('oauthClientID')"
              placeholder="…apps.googleusercontent.com"
              monospace
            />
            <OaTextField
              v-model="form.googleSecret"
              type="password"
              :label="t('oauthClientSecret')"
              :placeholder="form.googleSecretHint || '••••'"
              :hint="t('oauthCallback', { url: callbackURL('google') })"
              monospace
            />
          </div>
        </div>
        <OaSwitchField
          v-model="form.oauthAllowSignup"
          :label="t('oauthAllowSignup')"
          :hint="t('oauthAllowSignupHint')"
        />
        <OaSwitchField
          v-model="form.oauthLinkByEmail"
          :label="t('oauthLinkByEmail')"
          :hint="t('oauthLinkByEmailHint')"
        />
        <!-- Only worth showing where it would change anything: an instance
             that does not ask for a QQ number has nothing to exempt anybody
             from. -->
        <OaSwitchField
          v-if="form.qqRequirement === 'required'"
          v-model="form.oauthRequireQQ"
          :label="t('oauthRequireQQ')"
          :hint="t('oauthRequireQQHint')"
        />
      </AdminControlCard>
      <AdminControlCard id="secApplications" v-show="visible('secApplications')" :title="t('secApplications')" :icon="IconKey" :hint="t('applicationsHint')" class="oa-control-card-wide">
        <p class="oa-field-hint">{{ t('applicationsIssuer', { issuer }) }}</p>

        <p v-if="!applications.length" class="oa-table-empty">{{ t('applicationsEmpty') }}</p>
        <div v-else class="oa-apps">
          <div v-for="app in applications" :key="app.id" class="oa-app" :class="{ off: app.disabled }">
            <!-- Its initials, the same mark the consent screen draws it with,
                 so a row here and the card a stranger sees are the same thing. -->
            <span class="oa-app-mark">{{ initials(app.name) }}</span>
            <div class="oa-app-body">
              <div class="oa-app-head">
                <span class="oa-app-name">{{ app.name }}</span>
                <OaBadge v-if="app.disabled" tone="danger">{{ t('disabled') }}</OaBadge>
                <OaBadge v-if="app.trusted" tone="muted">{{ t('applicationTrusted') }}</OaBadge>
                <OaBadge v-if="!app.confidential" tone="muted">{{ t('applicationPublic') }}</OaBadge>
              </div>
              <p v-if="app.description" class="oa-app-desc">{{ app.description }}</p>
              <!-- The client id is the one value an operator has to move to
                   another program by hand, so the row is the copy control. -->
              <button type="button" class="oa-app-id mono" :title="t('copy')" @click="copy(app.client_id)">
                <span>{{ app.client_id }}</span>
                <IconCheck v-if="copied === app.client_id" :size="12" />
                <IconCopy v-else :size="12" />
              </button>
              <div class="oa-app-uris">
                <span v-for="uri in app.redirect_uris" :key="uri" class="oa-app-uri">{{ uri }}</span>
              </div>
            </div>
            <div class="oa-app-actions">
              <button type="button" class="oa-btn" :disabled="appBusy" @click="toggleApplication(app, !app.disabled)">
                {{ app.disabled ? t('enable') : t('disable') }}
              </button>
              <button v-if="app.confidential" type="button" class="oa-btn" :disabled="appBusy" @click="rotateApplication(app)">
                {{ t('applicationRotate') }}
              </button>
              <OaConfirmButton
                class="oa-btn"
                :label="t('deleteLabel')"
                :armed-label="t('confirmWord')"
                :armed-title="t('applicationDeleteConfirm', { application: app.name })"
                :resting-title="t('deleteLabel')"
                :disabled="appBusy"
                @confirm="removeApplication(app)"
              />
            </div>
          </div>
        </div>

        <!-- The one moment this value exists. It gets a surface of its own
             rather than a line of hint text, because everything else on this
             screen can be read again tomorrow and this cannot. -->
        <div v-if="freshSecret" class="oa-app-secret">
          <span class="oa-app-secret-label">{{ t('applicationSecretOnce') }}</span>
          <button type="button" class="oa-app-secret-value mono" :title="t('copy')" @click="copy(freshSecret)">
            <span>{{ freshSecret }}</span>
            <IconCheck v-if="copied === freshSecret" :size="13" />
            <IconCopy v-else :size="13" />
          </button>
        </div>

        <h3 class="oa-app-form-title">{{ t('applicationRegister') }}</h3>
        <OaTextField v-model="draft.name" :label="t('applicationName')" :hint="t('applicationNameHint')" />
        <OaTextField v-model="draft.description" :label="t('applicationDescription')" :hint="t('applicationDescriptionHint')" />
        <OaTextArea
          v-model="draft.redirects"
          :label="t('applicationRedirects')"
          :rows="2"
          placeholder="https://wiki.example.com/oidc/callback"
          :hint="t('applicationRedirectsHint')"
        />
        <OaSwitchField v-model="draft.public" :label="t('applicationPublicField')" :hint="t('applicationPublicHint')" />
        <OaSwitchField v-model="draft.trusted" :label="t('applicationTrustedField')" :hint="t('applicationTrustedHint')" />
        <div class="oa-button-row">
          <button type="button" class="oa-btn primary" :disabled="appBusy" @click="registerApplication">
            {{ t('applicationRegister') }}
          </button>
        </div>
        <p class="oa-drawer-flash" :class="{ visible: !!appFlash }">{{ appFlash }}</p>
      </AdminControlCard>
      <AdminControlCard id="secSecurityLog" v-show="visible('secSecurityLog')" :title="t('secSecurityLog')" :icon="IconFile" :hint="t('securityLogHint')" class="oa-control-card-wide">
        <button type="button" class="oa-btn" :disabled="eventsLoading" @click="loadEvents">
          {{ t('refresh') }}
        </button>
        <p v-if="eventsLoading" class="oa-table-empty">{{ t('loading') }}</p>
        <p v-else-if="!events.length" class="oa-table-empty">{{ t('securityLogEmpty') }}</p>
        <div v-else class="oa-log-list">
          <div v-for="event in events" :key="event.id" class="oa-log-row">
            <div class="oa-log-row-main">
              <div class="oa-log-row-head">
                <span class="oa-log-path">{{ eventLabel(event.event) }}</span>
                <OaBadge
                  :tone="event.severity === 'danger'
                    ? 'danger' : event.severity === 'warning' ? 'warning' : 'muted'"
                >{{ reviewDecision(event.decision ?? '') }}</OaBadge>
              </div>
              <div class="oa-log-row-meta">
                <span>{{ absoluteTime(event.at) }}</span>
                <span v-if="event.username">@{{ event.username }}</span>
                <span v-if="event.ip">{{ event.ip }}</span>
                <span v-if="event.actor_username">
                  {{ t('securityLogActor', { name: `@${event.actor_username}` }) }}
                </span>
              </div>
              <span v-if="event.reason" class="oa-field-hint">{{ event.reason }}</span>
            </div>
          </div>
        </div>
        <OaPagination v-bind="eventPage" :total="eventsTotal" :busy="eventsLoading" @change="changeEvents" />
      </AdminControlCard>
    </template>
  </AdminWorkbench>
  <p v-if="flash" class="oa-drawer-flash visible oa-control-flash" role="status">{{ flash }}</p>
</template>
