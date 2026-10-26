import { defineConfig, type Plugin } from "vite";
import { fileURLToPath } from "node:url";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";
import wails from "@wailsio/runtime/plugins/vite";

const mock = (file: string) => fileURLToPath(new URL(`./e2e/mock/${file}`, import.meta.url));

// --mode e2e (make e2e): the frontend runs in a plain browser against the fake
// backend in e2e/mock: the generated Service module and @wailsio/runtime are
// swapped for the mocks; the models stay the generated ones.
function e2eMocks(): Plugin {
  return {
    name: "wchron-e2e-mocks",
    enforce: "pre",
    resolveId(source, importer) {
      if (source === "@wailsio/runtime") return mock("runtime.ts");
      if (source === "./service.js" && importer && /[\\/]bindings[\\/].*[\\/]internal[\\/]app[\\/]index\.ts$/.test(importer)) {
        return mock("service.ts");
      }
      return null;
    },
  };
}

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => {
  const e2e = mode === "e2e";
  return {
    server: {
      host: "127.0.0.1",
      port: e2e ? 9246 : Number(process.env.WAILS_VITE_PORT) || 9245,
      strictPort: true,
      // The i18n catalog lives in ../locales, shared with Go.
      fs: { allow: [".", "../locales"] },
    },
    plugins: e2e ? [e2eMocks(), tailwindcss(), svelte()] : [tailwindcss(), svelte(), wails("./bindings")],
  };
});
