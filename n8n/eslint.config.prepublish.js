// Prepublish ESLint flat config (ESLint 9).
//
// Same rule set as eslint.config.js, but with the stricter
// `community-package-json-name-still-default` rule turned back ON, so a package that
// still carries the default placeholder name is caught before publishing.

const { buildConfig } = require('./eslint.config.js');

module.exports = buildConfig({ enforcePackageName: true });
