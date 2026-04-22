import { defineConfig } from "vite";

export default defineConfig({
  define: {
    "process.env.NODE_ENV": JSON.stringify("production"),
    "process.env": "{}"
  },
  build: {
    target: "es2022",
    outDir: "dist",
    emptyOutDir: true,
    lib: {
      entry: "src/index.ts",
      name: "OmadaNetworkCard",
      formats: ["es"],
      fileName: () => "omada-network-card.js"
    },
    rollupOptions: {
      output: {
        assetFileNames: "assets/[name]-[hash][extname]"
      }
    }
  }
});
