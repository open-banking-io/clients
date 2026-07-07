'use strict';

const { resolveBundle, apiRequest } = require('./lib/client');

// Custom auth: user pastes the credentials bundle JSON exported from open-banking.io.
// We parse it at auth-configuration time, extract the fields we need, and store
// them as connection-level authData (apiBaseUrl, apiKey, privateKey).
//
// The private key never leaves this Zapier connection — it is used only to
// decrypt data locally. The service only ever returns ciphertext it cannot read.

module.exports = {
  type: 'custom',
  test: {
    url: '{{bundle.authData.apiBaseUrl}}/api/connections',
    method: 'GET',
    headers: {
      'X-Api-Key': '{{bundle.authData.apiKey}}',
    },
  },
  fields: [
    {
      key: 'bundle',
      label: 'Credentials Bundle (JSON)',
      type: 'string',
      required: true,
      helpText:
        'Paste the credentials bundle JSON you exported from open-banking.io (or the CLI). ' +
        'It contains "apiBaseUrl", "apiKey" and "encryptionKey.privateKey". ' +
        'The private key never leaves this Zapier connection — it is used only to decrypt data locally.',
      computed: false,
      altersDynamicFields: false,
    },
    {
      key: 'baseUrlOverride',
      label: 'API Base URL Override',
      type: 'string',
      required: false,
      helpText:
        'Optional. Overrides the "apiBaseUrl" embedded in the bundle — e.g. to point ' +
        'at a staging or local environment. Leave empty to use the URL from the bundle.',
      computed: false,
      default: '',
      altersDynamicFields: false,
    },
    // Computed fields — extracted from the bundle at auth time, not shown to the user.
    {
      key: 'apiBaseUrl',
      type: 'string',
      required: false,
      computed: true,
    },
    {
      key: 'apiKey',
      type: 'string',
      required: false,
      computed: true,
    },
    {
      key: 'privateKey',
      type: 'string',
      required: false,
      computed: true,
    },
  ],
  customConfigurator: async (z, bundle) => {
    // Parse the bundle JSON, apply override, and return the computed fields.
    const resolved = resolveBundle({
      bundle: bundle.inputData.bundle,
      baseUrlOverride: bundle.inputData.baseUrlOverride,
    });

    return {
      apiBaseUrl: resolved.apiBaseUrl,
      apiKey: resolved.apiKey,
      privateKey: resolved.privateKey,
    };
  },
};
