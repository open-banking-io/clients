// The package version, sourced from package.json so it can never drift from the published
// manifest. `resolveJsonModule` lets tsc inline the field at build time and lets vitest resolve
// the import directly — mirroring how the node SDK derives its version (node/src/version.ts).
import pkg from '../../../package.json';

export const VERSION: string = pkg.version;

/** The `User-Agent` sent on every API request: `open-banking-io/n8n/<version>`. */
export const USER_AGENT = `open-banking-io/n8n/${VERSION}`;
