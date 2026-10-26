// Sets the app version in every file that holds it, then checks that the old
// version is left in none of them. Usage: node scripts/set-version.mjs x.y.z
// (or `make version V=x.y.z`). No dependencies: Node only.
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const next = process.argv[2] ?? "";
if (!/^\d+\.\d+\.\d+$/.test(next)) {
  console.error(`usage: node scripts/set-version.mjs x.y.z (got "${next}")`);
  process.exit(1);
}

const read = (f) => readFileSync(join(root, f), "utf8");
const write = (f, s) => writeFileSync(join(root, f), s);

const configMatch = /^\s+version:\s*"(\d+\.\d+\.\d+)"/m.exec(read("build/config.yml"));
if (!configMatch) {
  console.error("build/config.yml: no version found");
  process.exit(1);
}
const old = configMatch[1];
if (old === next) {
  console.log(`already at ${next}`);
  process.exit(0);
}
const q = old.replaceAll(".", "\\.");

// [file, pattern, replacement]: each pattern must match at least once.
const edits = [
  ["build/config.yml", new RegExp(`(\\n\\s+version:\\s*")${q}(")`), `$1${next}$2`],
  ["build/windows/info.json", new RegExp(`"${q}"`, "g"), `"${next}"`],
  ["build/windows/nsis/wails_tools.nsh", new RegExp(`(INFO_PRODUCTVERSION ")${q}(")`), `$1${next}$2`],
  // the manifest wants four numbers
  ["build/windows/wails.exe.manifest", new RegExp(`(version=")${q}\\.0(")`), `$1${next}.0$2`],
  ["internal/version/version.go", new RegExp(`(const Version = ")${q}(")`), `$1${next}$2`],
  ["frontend/package.json", new RegExp(`("version": ")${q}(")`), `$1${next}$2`],
];

for (const [file, re, repl] of edits) {
  const s = read(file);
  if (!re.test(s)) {
    console.error(`${file}: version ${old} not found`);
    process.exit(1);
  }
  re.lastIndex = 0;
  write(file, s.replace(re, repl));
}

// package-lock.json: only the app's own two entries change, so they are edited
// directly instead of `npm install --package-lock-only` (npm 12 refuses to
// re-resolve some optional tarballs in lock-only mode).
{
  const lock = JSON.parse(read("frontend/package-lock.json"));
  if (lock.version !== old || lock.packages?.[""]?.version !== old) {
    console.error(`frontend/package-lock.json: version ${old} not found`);
    process.exit(1);
  }
  lock.version = next;
  lock.packages[""].version = next;
  write("frontend/package-lock.json", JSON.stringify(lock, null, 2) + "\n");
}

// The old version (as a whole: 0.1.9 must not match 0.1.91) must be gone from
// the version files. package-lock.json: only the app's own entries, not the
// dependencies, which may share the number.
const whole = new RegExp(`(^|[^0-9.])${q}([^0-9]|$)`, "m");
const left = edits.map(([f]) => f).filter((f) => whole.test(read(f)));
const lockHead = read("frontend/package-lock.json").split("\n").slice(0, 12).join("\n");
if (whole.test(lockHead)) left.push("frontend/package-lock.json");
if (left.length > 0) {
  console.error(`version ${old} still present in: ${left.join(", ")}`);
  process.exit(1);
}
console.log(`${old} -> ${next}`);
