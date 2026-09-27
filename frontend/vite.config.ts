import { mkdirSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { defineConfig, type Plugin } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import wails from "@wailsio/runtime/plugins/vite";

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
  test: {
    setupFiles: ["./vitest.setup.ts"],
  },
});
