import fs from "node:fs";
import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import wails from "@wailsio/runtime/plugins/vite";

// Vite empties dist/ on build. Restore the committed placeholder so
// //go:embed all:frontend/dist still succeeds after a production bundle.
function preserveDistGitkeep() {
  return {
    name: "preserve-dist-gitkeep",
    closeBundle() {
      const gitkeep = path.resolve(import.meta.dirname, "dist/.gitkeep");
      fs.mkdirSync(path.dirname(gitkeep), { recursive: true });
      fs.writeFileSync(gitkeep, "");
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
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  plugins: [react(), tailwindcss(), wails("./bindings"), preserveDistGitkeep()],
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          // echarts stays a dedicated vendor chunk so the main index file
          // stays well under Vite's 500 kB warning. Overview still imports
          // charts statically, so this chunk loads in parallel with no
          // landing-page Suspense flash. The remaining >500 kB warning is
          // this vendor file (~588 kB), a measured exception.
          if (id.includes("node_modules/echarts") || id.includes("node_modules/zrender")) {
            return "echarts";
          }
          return undefined;
        },
      },
    },
  },
});
