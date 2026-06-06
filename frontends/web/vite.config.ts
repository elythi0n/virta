import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// fs.allow opens the sibling ui-kit/ so the dev server can serve the shared, generated
// tokens.css (the single source of design tokens) from outside this root.
export default defineConfig({
  plugins: [react()],
  // The ui-kit workspace package shares this app's React; dedupe avoids a second copy
  // (which would break Radix's hooks/context).
  resolve: { dedupe: ['react', 'react-dom'] },
  server: { fs: { allow: ['..'] } },
});
