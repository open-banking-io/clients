import type {
	IAuthenticateGeneric,
	ICredentialTestRequest,
	ICredentialType,
	INodeProperties,
} from 'n8n-workflow';

export class OpenBankingIoApi implements ICredentialType {
	name = 'openBankingIoApi';

	displayName = 'Open Banking IO API';

	documentationUrl = 'https://open-banking.io';

	properties: INodeProperties[] = [
		{
			displayName: 'Credentials Bundle (JSON)',
			name: 'bundle',
			type: 'string',
			typeOptions: {
				password: true,
				rows: 8,
			},
			default: '',
			required: true,
			description:
				'Paste the credentials bundle JSON you exported from open-banking.io (or the CLI). It contains "apiBaseUrl", "apiKey" and "encryptionKey.privateKey". The private key never leaves this n8n instance — it is used only to decrypt data locally.',
		},
		{
			displayName: 'API Base URL Override',
			name: 'baseUrlOverride',
			type: 'string',
			default: '',
			placeholder: 'https://api.staging.open-banking.io',
			description:
				'Optional. Overrides the "apiBaseUrl" embedded in the bundle — e.g. to point this credential at a staging or local environment. Leave empty to use the URL from the bundle.',
		},
	];

	// The API key lives inside the pasted bundle, so we parse it out of the JSON at request time
	// and send it as the X-Api-Key header. The private key is never injected here.
	authenticate: IAuthenticateGeneric = {
		type: 'generic',
		properties: {
			headers: {
				'X-Api-Key': '={{ JSON.parse($credentials.bundle).apiKey }}',
			},
		},
	};

	// Lets the user click "Test" in the credential UI: lists connections with the parsed key.
	// Prefers the optional baseUrlOverride; falls back to the bundle's apiBaseUrl.
	test: ICredentialTestRequest = {
		request: {
			baseURL:
				'={{ (($credentials.baseUrlOverride || "").trim() || JSON.parse($credentials.bundle).apiBaseUrl).replace(/\\/+$/, "") }}',
			url: '/api/connections',
			method: 'GET',
		},
	};
}
