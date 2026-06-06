// Token lint: component styles must use design tokens, never raw hex colors, and every
// var(--virta-*) they reference must actually be defined — an undefined token silently voids the
// whole declaration (e.g. a bad value in a `padding` shorthand drops all of it), so this guards a
// class of invisible layout bugs. The generated token artifacts (ui-kit/tokens.css, tokens.ts) are
// the one place colors are literal and are the source of truth for defined tokens. Run from the
// frontends/ workspace root.
import { globSync, readFileSync } from 'node:fs';

const files = globSync([
  'ui-kit/src/**/*.{css,ts,tsx}',
  'feed-core/src/**/*.{css,ts,tsx}',
  'web/src/**/*.{css,ts,tsx}',
]).filter((f) => !/\.(test|spec)\.[tj]sx?$/.test(f)); // test fixtures may use literal colors
const HEX = /#[0-9a-fA-F]{3,8}\b/g;
const DEF = /--virta-[\w-]+(?=\s*:)/g; // a token definition: `--virta-x:`
const REF = /var\(\s*(--virta-[\w-]+)/g; // a token reference: `var(--virta-x`

// Defined tokens come from the generated artifacts plus any --virta-* declared in scanned files.
const defined = new Set();
const defSources = ['ui-kit/tokens.css', ...files];
for (const f of defSources) {
  let text;
  try {
    text = readFileSync(f, 'utf8');
  } catch {
    continue;
  }
  for (const m of text.matchAll(DEF)) defined.add(m[0]);
}

const violations = [];
for (const file of files.sort()) {
  const lines = readFileSync(file, 'utf8').split('\n');
  lines.forEach((line, i) => {
    for (const m of line.matchAll(HEX)) {
      violations.push(`${file}:${i + 1}  raw hex ${m[0]}  ->  ${line.trim()}`);
    }
    for (const m of line.matchAll(REF)) {
      if (!defined.has(m[1])) {
        violations.push(`${file}:${i + 1}  undefined token ${m[1]}  ->  ${line.trim()}`);
      }
    }
  });
}

if (violations.length > 0) {
  console.error('token-lint: violations (use defined var(--virta-*) tokens, no raw hex):\n');
  for (const v of violations) console.error('  ' + v);
  console.error(`\n${violations.length} violation(s).`);
  process.exit(1);
}
console.log(`token-lint: ${files.length} files clean (tokens defined, no raw hex).`);
