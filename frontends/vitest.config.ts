import { defineConfig } from 'vitest/config';

// Unit tests for the frontend packages. Pure logic (parsers, color math) runs in the node
// environment; component/DOM tests can opt into jsdom per-file when they arrive.
export default defineConfig({
  test: {
    environment: 'node',
    include: ['*/src/**/*.test.ts'],
  },
});
