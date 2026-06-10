// The package version, sourced from package.json so it can never drift. tsup inlines the JSON at
// build time (tree-shaken to the single string), and vitest/tsx resolve the import directly — so
// this stays ESM/CJS-safe without a runtime file read.
import pkg from "../package.json" with { type: "json" };

export const VERSION: string = pkg.version;

/** The `User-Agent` sent on every request: `open-banking-io/node/<version>`. */
export const USER_AGENT = `open-banking-io/node/${VERSION}`;
