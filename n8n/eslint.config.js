// ESLint flat config (ESLint 9/10).
//
// ESLint 10 removed support for the legacy `.eslintrc.*` format entirely, so this
// file replaces the former `.eslintrc.js`. `eslint-plugin-n8n-nodes-base` (1.16.x)
// still ships only legacy-style shareable configs, so we cannot `extends` them from
// flat config; instead we attach the plugin object directly and reuse its published
// rule sets (`configs.community` / `configs.credentials` / `configs.nodes`). The rule
// coverage is therefore identical to the previous `.eslintrc.js`, which keeps the n8n
// community-node scanner / lint gate satisfied.

const parser = require('@typescript-eslint/parser');
const n8nNodesBase = require('eslint-plugin-n8n-nodes-base');

/**
 * Whether the stricter "package name must not be the default" rule is enforced.
 * Off for normal lint, on for prepublish (see eslint.config.prepublish.js).
 */
function buildConfig({ enforcePackageName = false } = {}) {
	return [
		{
			ignores: [
				'eslint.config.js',
				'eslint.config.prepublish.js',
				'gulpfile.js',
				'**/*.js',
				'**/node_modules/**',
				'**/dist/**',
			],
		},
		// package.json — community metadata rules. Parsed with the TS parser, which the
		// n8n package.json rules expect (mirrors the old `extraFileExtensions: ['.json']`).
		{
			files: ['package.json'],
			plugins: { 'n8n-nodes-base': n8nNodesBase },
			languageOptions: {
				parser,
				sourceType: 'module',
				parserOptions: { extraFileExtensions: ['.json'] },
			},
			rules: {
				...n8nNodesBase.configs.community.rules,
				'n8n-nodes-base/community-package-json-name-still-default': enforcePackageName
					? 'error'
					: 'off',
			},
		},
		// credentials/**/*.ts — credentials rules.
		{
			files: ['credentials/**/*.ts'],
			plugins: { 'n8n-nodes-base': n8nNodesBase },
			languageOptions: {
				parser,
				sourceType: 'module',
				parserOptions: { project: ['./tsconfig.json'] },
			},
			rules: {
				...n8nNodesBase.configs.credentials.rules,
				// Main-repo-only rule: it wants a camelCased doc slug, which conflicts with the
				// community rule requiring a full http(s) URL. Community packages use a real URL.
				'n8n-nodes-base/cred-class-field-documentation-url-miscased': 'off',
			},
		},
		// nodes/**/*.ts — node rules.
		{
			files: ['nodes/**/*.ts'],
			plugins: { 'n8n-nodes-base': n8nNodesBase },
			languageOptions: {
				parser,
				sourceType: 'module',
				parserOptions: { project: ['./tsconfig.json'] },
			},
			rules: {
				...n8nNodesBase.configs.nodes.rules,
			},
		},
	];
}

const config = buildConfig();
config.buildConfig = buildConfig;
module.exports = config;
