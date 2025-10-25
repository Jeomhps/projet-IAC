import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In dev, you can optionally proxy to API if you run `vite` directly.
// But for dockerized usage, Caddy serves static and proxies /api.
// Uncomment below to use Vite dev server proxy.
//
// const devProxy = {
//   "/api": {
//     target: "https://localhost",
//     changeOrigin: true,
//     secure: false
//   }
// };

export default defineConfig({
  plugins: [react()],
  // server: { proxy: devProxy }
});
