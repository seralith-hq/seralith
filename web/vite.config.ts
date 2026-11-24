import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

const daemon = process.env.SIGIL_DAEMON ?? "http://127.0.0.1:7457";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": { target: daemon, ws: true, changeOrigin: false },
    },
  },
  build: {
    outDir: "dist",
    sourcemap: false,
    chunkSizeWarningLimit: 900,
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
