import type { NextConfig } from "next";

const backend = process.env.BACKEND_URL ?? "http://localhost:8080";
const isProd = process.env.NODE_ENV === "production";

// Content Security Policy. Next.js hydration requires inline scripts unless every page is rendered
// dynamically with a nonce; we keep pages cacheable and allow 'unsafe-inline' for scripts only, with
// everything else locked to our own origin. The 3D loaders need WebAssembly (Draco, Basis) and
// blob: workers.
const csp = [
  "default-src 'self'",
  `script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval'${isProd ? "" : " 'unsafe-eval'"}`,
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob:",
  "font-src 'self'",
  "connect-src 'self' blob: data:",
  "worker-src 'self' blob:",
  "media-src 'self' blob:",
  "frame-src 'self' https://www.youtube-nocookie.com https://www.google.com",
  "frame-ancestors 'none'",
  "base-uri 'self'",
  "form-action 'self'",
  "object-src 'none'",
  ...(isProd ? ["upgrade-insecure-requests"] : []),
].join("; ");

const securityHeaders = [
  { key: "Content-Security-Policy", value: csp },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=(), payment=()" },
  ...(isProd ? [{ key: "Strict-Transport-Security", value: "max-age=63072000; includeSubDomains" }] : []),
];

const config: NextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,
  agentRules: false,
  output: "standalone",
  images: {
    formats: ["image/avif", "image/webp"],
    localPatterns: [
      { pathname: "/api/v1/uploads/**" },
      { pathname: "/textures/**" },
      { pathname: "/images/**" },
      { pathname: "/brand/**" },
      { pathname: "/3d/**" },
    ],
    deviceSizes: [360, 480, 640, 828, 1080, 1280, 1600, 1920],
    minimumCacheTTL: 86400,
  },
  async rewrites() {
    return [{ source: "/api/v1/:path*", destination: `${backend}/api/v1/:path*` }];
  },
  async headers() {
    return [
      { source: "/:path*", headers: securityHeaders },
      { source: "/3d/:path*", headers: [{ key: "Cache-Control", value: "public, max-age=31536000, immutable" }] },
      {
        source: "/textures/:path*",
        headers: [{ key: "Cache-Control", value: "public, max-age=604800, stale-while-revalidate=86400" }],
      },
    ];
  },
};

export default config;
