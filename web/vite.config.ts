import { defineConfig } from "vite";
import solid from "vite-plugin-solid";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "node:path";

export default defineConfig({
  plugins: [tailwindcss(), solid()],
  resolve: {
    alias: {
      // import.meta.dirname, not __dirname: Vite 8 warns that __dirname is
      // unsupported by its next-generation native config loader.
      "@": resolve(import.meta.dirname, "./src"),
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
