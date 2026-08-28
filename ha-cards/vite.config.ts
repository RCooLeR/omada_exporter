import { defineConfig } from "vite";

export default defineConfig({
  define: {
    "process.env.NODE_ENV": JSON.stringify("production")
  },
  build: {
    target: "baseline-widely-available",
    outDir: "dist",
    emptyOutDir: true,
    lib: {
      entry: "src/index.ts",
      name: "OmadaNetworkCard",
      formats: ["es"],
      fileName: () => "omada-network-card.js"
    },
    rolldownOptions: {
      output: {
        assetFileNames: "assets/[name]-[hash][extname]"
      }
    }
  }
});
