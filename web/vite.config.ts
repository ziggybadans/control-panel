import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Build output goes straight into the Go embed directory so `make build`
// produces a single self-contained binary.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../internal/httpd/ui/dist",
    emptyOutDir: true,
    sourcemap: false,
    target: "es2020",
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:9090",
        changeOrigin: false,
      },
    },
  },
});
