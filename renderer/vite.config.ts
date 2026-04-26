import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "node:path";

// The renderer builds a single bundle that the Go server embeds via
// //go:embed in server/assets/embed.go. Output lands in
// ../server/assets/dist relative to this file.

export default defineConfig({
  plugins: [svelte(), tailwindcss()],

  build: {
    outDir: resolve(__dirname, "../server/assets/dist"),
    emptyOutDir: true,
    sourcemap: true,
    rollupOptions: {
      input: resolve(__dirname, "src/main.ts"),
      output: {
        entryFileNames: "index.js",
        chunkFileNames: "chunks/[name]-[hash].js",
        assetFileNames: (info) => {
          if (info.name && info.name.endsWith(".css")) {
            return "index.css";
          }
          return "assets/[name]-[hash][extname]";
        },
      },
    },
  },

  server: {
    port: 5173,
    strictPort: false,
  },
});
