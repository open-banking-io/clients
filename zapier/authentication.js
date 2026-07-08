'use strict';

const { resolveBundle, apiRequest } = require('./lib/client');

// Custom auth: user pastes the credentials bundle JSON exported from open-banking.io.
// The bundle (plus the optional base-URL override) is stored as authData and
// re-parsed on every call via resolveBundle() — custom auth does not support
// computed fields, so nothing is extracted at auth time.
//
// The private key never leaves this Zapier connection — it is used only to
// decrypt data locally. The service only ever returns ciphertext it cannot read.

module.exports = {
  type: 'custom',
  test: async (z, bundle) => {
    // Parses/validates the pasted bundle, then verifies the credentials
    // against the API. z.request throws on any >= 400 response.
    const resolved = resolveBundle(bundle.authData);
    const connections = await apiRequest(z, resolved, 'GET', '/api/connections');
    return {
      connectionCount: Array.isArray(connections) ? connections.length : 0,
    };
  },
  connectionLabel: 'open-banking.io ({{bundle.inputData.connectionCount}} bank connection(s))',
  fields: [
    {
      key: 'bundle',
      label: 'Credentials Bundle (JSON)',
      type: 'text',
      required: true,
      helpText:
        'Paste the credentials bundle JSON you exported from open-banking.io (or the CLI). ' +
        'It contains "apiBaseUrl", "apiKey" and "encryptionKey.privateKey". ' +
        'The private key never leaves this Zapier connection — it is used only to decrypt data locally. ' +
        'See the [setup guide](https://github.com/open-banking-io/zapier#credentials) for how to export the bundle.',
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
        'at a staging or self-hosted environment. Must be a full http(s) URL. Leave empty ' +
        'to use the URL from the bundle. See the ' +
        '[setup guide](https://github.com/open-banking-io/zapier#credentials) for details.',
      computed: false,
      altersDynamicFields: false,
    },
  ],
};
