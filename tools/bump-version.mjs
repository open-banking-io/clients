#!/usr/bin/env node
// bump-version.mjs — set a package's manifest version in this monorepo.
//
//   node tools/bump-version.mjs <package> <version>
//
// <package> ∈ dotnet | node | n8n | python | rust | java | ruby | beancount   (go and php have no manifest)
// <version> is a plain semver string, e.g. 0.2.0  or  1.0.0-rc.1
//
// Plain Node (>= 20), no dependencies. Edits the version field in the package's
// manifest (and any lockfiles that pin this package's own version). The matching
// publish-<package>.yml gate then asserts tag == manifest before publishing.

import { readFileSync, writeFileSync, existsSync, readdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join, resolve } from "node:path";

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, "..");

const PACKAGES = ["dotnet", "node", "n8n", "python", "rust", "java", "go", "ruby", "php", "beancount", "erpnext"];

// SemVer 2.0.0 (https://semver.org) — practical, anchored.
const SEMVER =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$/;

function die(msg) {
  console.error(`bump-version: ${msg}`);
  process.exit(1);
}

function read(rel) {
  const p = join(ROOT, rel);
  if (!existsSync(p)) die(`expected file not found: ${rel}`);
  return readFileSync(p, "utf8");
}

function write(rel, content) {
  writeFileSync(join(ROOT, rel), content);
  console.log(`  updated ${rel}`);
}

// Replace exactly one occurrence matched by `re` (the version capture is group 1),
// failing loudly if the field isn't found — we never want a silent no-op release.
function replaceOne(rel, re, version, label) {
  const src = read(rel);
  if (!re.test(src)) die(`could not find ${label} in ${rel}`);
  re.lastIndex = 0;
  const out = src.replace(re, (m, before) => `${before}${version}`);
  write(rel, out);
}

const bumpers = {
  dotnet(version) {
    // <Version>0.1.0</Version> in the client csproj.
    replaceOne(
      "dotnet/src/OpenBankingIO.Client/OpenBankingIO.Client.csproj",
      /(<Version>)[^<]+(?=<\/Version>)/,
      version,
      "<Version>",
    );
  },

  node(version) {
    // package.json + package-lock.json both pin "version".
    setJsonVersion("node/package.json", version);
    setLockfileVersion("node/package-lock.json", version);
  },

  n8n(version) {
    // The n8n community node package — same shape as node/.
    setJsonVersion("n8n/package.json", version);
    setLockfileVersion("n8n/package-lock.json", version);
  },

  python(version) {
    // [project] version = "0.1.0" in pyproject.toml. Anchor to line start so we
    // don't accidentally hit requires-python or a dependency spec.
    replaceOne(
      "python/pyproject.toml",
      /(^version[ \t]*=[ \t]*")[^"]+(?="$)/m,
      version,
      'version = "..." (under [project])',
    );
  },

  beancount(version) {
    // The Beancount integration package — same pyproject shape as python/.
    replaceOne(
      "beancount/pyproject.toml",
      /(^version[ \t]*=[ \t]*")[^"]+(?="$)/m,
      version,
      'version = "..." (under [project])',
    );
  },

  erpnext(version) {
    // The ERPNext (Frappe) app — pyproject [project] version plus the
    // app package's __version__ (Frappe reads it for the app metadata).
    replaceOne(
      "erpnext/pyproject.toml",
      /(^version[ \t]*=[ \t]*")[^"]+(?="$)/m,
      version,
      'version = "..." (under [project])',
    );
    replaceOne(
      "erpnext/erpnext_open_banking/__init__.py",
      /(^__version__[ \t]*=[ \t]*")[^"]+(?="$)/m,
      version,
      '__version__ = "..."',
    );
  },

  rust(version) {
    // [package] version = "0.1.0" — the first `version = ` after [package].
    setRustPackageVersion("rust/Cargo.toml", version);
    setCargoLockVersion("rust/Cargo.lock", "open-banking-io", version);
  },

  java(version) {
    // The project <version> is the first <version> element, before <dependencies>.
    setMavenProjectVersion("java/pom.xml", version);
  },

  ruby(version) {
    setRubyGemVersion(version);
    // Keep the gem's own pin in Gemfile.lock in sync (two occurrences) so
    // bundler's frozen install in CI doesn't reject the bump.
    const rel = "ruby/Gemfile.lock";
    if (existsSync(join(ROOT, rel))) {
      const src = read(rel);
      const out = src.replace(/(open-banking-io \()[0-9][^)]*(\))/g, `$1${version}$2`);
      if (out !== src) write(rel, out);
    }
  },

  go() {
    die(
      'package "go" has no manifest — its version IS the git tag (go/vX.Y.Z). ' +
        "Create the tag directly; do not run bump-version for go.",
    );
  },

  php() {
    die(
      'package "php" has no manifest version — Packagist derives it from the git ' +
        "tag (php/vX.Y.Z). Create the tag directly; do not run bump-version for php.",
    );
  },
};

// ---- node helpers ---------------------------------------------------------

function setJsonVersion(rel, version) {
  // String-replace the top-level "version" so we preserve formatting/key order.
  replaceOne(
    rel,
    /("version"[ \t]*:[ \t]*")[^"]+(?=")/,
    version,
    `"version" in ${rel}`,
  );
}

function setLockfileVersion(rel, version) {
  // npm lockfile v2/v3: the root "version" near the top, and the root package
  // entry packages[""].version. Both should track the manifest.
  const src = read(rel);
  let out = src;
  let changed = false;

  // Top-level "version" (first occurrence, before any "packages"/"dependencies").
  out = out.replace(/("version"[ \t]*:[ \t]*")[^"]+(")/, (m, a, b) => {
    changed = true;
    return `${a}${version}${b}`;
  });

  // packages[""].version — the root package entry.
  out = out.replace(
    /("":[ \t]*\{[\s\S]*?"version"[ \t]*:[ \t]*")[^"]+(")/,
    (m, a, b) => {
      changed = true;
      return `${a}${version}${b}`;
    },
  );

  if (!changed) die(`could not find a "version" field in ${rel}`);
  write(rel, out);
}

// ---- rust helpers ---------------------------------------------------------

function setRustPackageVersion(rel, version) {
  const src = read(rel);
  const pkgIdx = src.indexOf("[package]");
  if (pkgIdx === -1) die(`no [package] table in ${rel}`);
  // Only search within the [package] table (up to the next table header).
  const after = src.slice(pkgIdx);
  const nextTable = after.indexOf("\n[", 1);
  const end = nextTable === -1 ? src.length : pkgIdx + nextTable;
  const head = src.slice(0, pkgIdx);
  const body = src.slice(pkgIdx, end);
  const tail = src.slice(end);

  const re = /(^version[ \t]*=[ \t]*")[^"]+(?="$)/m;
  if (!re.test(body)) die(`no version field in [package] table of ${rel}`);
  const newBody = body.replace(re, (m, a) => `${a}${version}`);
  write(rel, head + newBody + tail);
}

function setCargoLockVersion(rel, pkgName, version) {
  const src = read(rel);
  // Cargo.lock is a series of [[package]] blocks, each formatted canonically as
  // `name = "..."` immediately followed by `version = "..."`. Locate the target
  // block by a literal string search (no dynamic RegExp -> no ReDoS surface) and
  // rewrite only that version value.
  const needle = `name = "${pkgName}"\nversion = "`;
  const nameIdx = src.indexOf(needle);
  if (nameIdx === -1) die(`no [[package]] "${pkgName}" entry in ${rel}`);
  const valStart = nameIdx + needle.length;
  const valEnd = src.indexOf('"', valStart);
  if (valEnd === -1) die(`malformed version for "${pkgName}" in ${rel}`);
  write(rel, src.slice(0, valStart) + version + src.slice(valEnd));
}

// ---- java helper ----------------------------------------------------------

function setMavenProjectVersion(rel, version) {
  const src = read(rel);
  // The project version is the first <version> that appears before <dependencies>.
  // (Dependency/plugin <version>s come later.) Replace exactly that first one.
  const depIdx = src.indexOf("<dependencies>");
  const scope = depIdx === -1 ? src : src.slice(0, depIdx);
  const re = /(<version>)[^<]+(<\/version>)/;
  if (!re.test(scope)) die(`no project <version> found in ${rel}`);
  const newScope = scope.replace(re, (m, a, b) => `${a}${version}${b}`);
  const out = newScope + (depIdx === -1 ? "" : src.slice(depIdx));
  write(rel, out);
}

// ---- ruby helper ----------------------------------------------------------

function setRubyGemVersion(version) {
  // Ruby gems declare their version in one of two idiomatic ways:
  //   1) directly in the gemspec:  spec.version = "0.1.0"
  //   2) via a VERSION constant in lib/**/version.rb, referenced by the gemspec.
  // Update wherever the version actually lives. A gemspec with a literal version
  // wins; otherwise fall back to the VERSION constant (which is the source of
  // truth when the gemspec interpolates it).
  const rubyDir = join(ROOT, "ruby");
  if (!existsSync(rubyDir)) die("ruby/ directory not found");

  // Case 1: a literal version assignment in the gemspec (if a gemspec exists).
  const gemspec = readdirSync(rubyDir).find((f) => f.endsWith(".gemspec"));
  if (gemspec) {
    const gemspecRel = `ruby/${gemspec}`;
    const gemspecSrc = read(gemspecRel);
    const litRe = /(\.version\s*=\s*["'])[^"']+(["'])/;
    if (litRe.test(gemspecSrc)) {
      const out = gemspecSrc.replace(litRe, (m, a, b) => `${a}${version}${b}`);
      write(gemspecRel, out);
      return;
    }
  }

  // Case 2: a VERSION constant somewhere under ruby/lib (e.g. version.rb).
  const versionFile = findRubyVersionFile(join(rubyDir, "lib"));
  if (versionFile) {
    const rel = versionFile.slice(ROOT.length + 1);
    const src = read(rel);
    const constRe = /(VERSION\s*=\s*["'])[^"']+(["'])/;
    if (constRe.test(src)) {
      const out = src.replace(constRe, (m, a, b) => `${a}${version}${b}`);
      write(rel, out);
      return;
    }
  }

  die(
    `could not locate a version declaration for the ruby gem ` +
      `(looked for \`.version =\` in ${gemspecRel} and a \`VERSION =\` constant under ruby/lib)`,
  );
}

function findRubyVersionFile(dir) {
  if (!existsSync(dir)) return null;
  const stack = [dir];
  let fallback = null;
  while (stack.length) {
    const cur = stack.pop();
    for (const ent of readdirSync(cur, { withFileTypes: true })) {
      const full = join(cur, ent.name);
      if (ent.isDirectory()) stack.push(full);
      else if (ent.name === "version.rb") return full;
      else if (ent.isFile() && ent.name.endsWith(".rb") && !fallback) {
        // Remember any .rb that defines a VERSION constant as a fallback.
        try {
          if (/VERSION\s*=\s*["']/.test(readFileSync(full, "utf8")))
            fallback = full;
        } catch {}
      }
    }
  }
  return fallback;
}

// ---- main -----------------------------------------------------------------

function main() {
  const [pkg, version] = process.argv.slice(2);

  if (!pkg || !version) {
    die("usage: node tools/bump-version.mjs <package> <version>");
  }
  if (!PACKAGES.includes(pkg)) {
    die(`unknown package "${pkg}" (expected one of: ${PACKAGES.join(", ")})`);
  }
  if (!SEMVER.test(version)) {
    die(`"${version}" is not a valid semver version (e.g. 0.2.0)`);
  }

  console.log(`bump-version: ${pkg} -> ${version}`);
  bumpers[pkg](version);
  console.log("done.");
}

main();
