export type CCSwitchApp = 'claude' | 'codex';

export interface CCSwitchProvider {
  app: CCSwitchApp;
  origin: string;
  name: string;
  token: string;
  model: string;
}

export function ccSwitchImportURL(provider: CCSwitchProvider): string {
  const origin = new URL(provider.origin).origin;
  const params = new URLSearchParams({
    resource: 'provider',
    app: provider.app,
    name: provider.name,
    homepage: origin,
    // Claude appends /v1/messages, whereas Codex appends /responses.
    endpoint: provider.app === 'claude' ? origin : `${origin}/v1`,
    apiKey: provider.token,
    model: provider.model,
    enabled: 'false',
  });
  if (provider.app === 'claude') {
    // Client aliases must not escape the model restrictions on this key.
    for (const alias of ['haikuModel', 'sonnetModel', 'opusModel']) {
      params.set(alias, provider.model);
    }
  }
  return `ccswitch://v1/import?${params}`;
}

export function openCCSwitch(provider: CCSwitchProvider): void {
  // Build only on a user gesture: no persistent link containing the secret,
  // intermediary website, or automatic replacement of the active provider.
  window.location.assign(ccSwitchImportURL(provider));
}
