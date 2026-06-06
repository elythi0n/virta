// Token lint: component styles must use design tokens, never raw hex colors. The generated token
// artifacts (ui-kit/tokens.css, tokens.ts) are the one place colors are literal and are not
// scanned. Run from the frontends/ workspace root.
import { globSync, readFileSync } from 'node:fs';

const files = globSync(['ui-kit/src/**/*.{css,ts,tsx}', 'web/src/**/*.{css,ts,tsx}']);
const HEX = /#[0-9a-fA-F]{3,8}\b/g;

const violations = [];
for (const file of files.sort()) {
  const lines = readFileSync(file, 'utf8').split('\n');
  lines.forEach((line, i) => {
    for (const m of line.matchAll(HEX)) {
      violations.push(`${file}:${i + 1}  ${m[0]}  ->  ${line.trim()}`);
    }
  });
}

if (violations.length > 0) {
  console.error('token-lint: raw hex colors found (use var(--virta-*) tokens):\n');
  for (const v of violations) console.error('  ' + v);
  console.error(`\n${violations.length} violation(s).`);
  process.exit(1);
}
console.log(`token-lint: ${files.length} files clean (no raw hex colors).`);
