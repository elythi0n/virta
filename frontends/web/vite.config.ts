import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// fs.allow opens the sibling ui-kit/ so the dev server can serve the shared,
// generated tokens.css (the single source of design tokens) from outside this root.
export default defineConfig({
  plugins: [svelte()],
  server: { fs: { allow: ['..'] } },
});
