import { mkdirSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { defineConfig, type Plugin } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import wails from "@wailsio/runtime/plugins/vite";
import { svelteTesting } from "@testing-library/svelte/vite";
import { configDefaults } from "vitest/config";

const DOM_TESTS = "src/**/*.dom.test.ts";

// dist очищается при каждой сборке, а //go:embed all:frontend/dist требует
// существования каталога в свежем клоне до первой сборки фронтенда.
function keepDist(): Plugin {
  return {
    name: "typhon-keep-dist",
    closeBundle() {
      const dir = resolve(__dirname, "dist");
      mkdirSync(dir, { recursive: true });
      writeFileSync(resolve(dir, ".gitkeep"), "");
    },
  };
}

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [svelte(), wails("./bindings"), keepDist()],
  // A test that needs a DOM says so in its name; everything else stays in node and renders through
  // svelte/server. The name is the only switch: domConvention.test.ts rejects a per-file
  // `@vitest-environment` docblock.
  test: {
    projects: [
      {
        extends: true,
        test: {
          name: "node",
          exclude: [...configDefaults.exclude, DOM_TESTS],
          setupFiles: ["./vitest.setup.ts"],
        },
      },
      {
        extends: true,
        // Resolves `svelte` to the client runtime for this project only; the cleanup hook is
        // imported by vitest.dom.setup.ts, not added to every project's setup files.
        plugins: [svelteTesting({ autoCleanup: false })],
        test: {
          name: "dom",
          include: [DOM_TESTS],
          environment: "jsdom",
          setupFiles: ["./vitest.dom.setup.ts"],
        },
      },
    ],
  },
});
