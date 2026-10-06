import type { NextConfig } from "next";

const backendUrl = process.env.BACKEND_URL || "http://127.0.0.1:8090";

// The dev server refuses cross-origin dev resources (HMR socket, stack frames) unless the
// origin is listed. Open it to private LAN addresses so http://192.168.x.x:8080 works;
// add more with ALLOWED_DEV_ORIGINS=host1,host2.
const allowedDevOrigins = [
  "192.168.*.*",
  "10.*.*.*",
  "172.16.*.*",
  "*.local",
  ...(process.env.ALLOWED_DEV_ORIGINS || "").split(",").map((origin) => origin.trim()).filter(Boolean),
];

const nextConfig: NextConfig = {
  output: "standalone",
  allowedDevOrigins,
  // Hydration must keep working when a browser/proxy cannot connect the dev
  // WebSocket carrying React's optional server debugging stream.
  experimental: { reactDebugChannel: false },
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          // Clickjacking and plugin/base-tag hardening; no script-src yet (it needs nonces for Next's inline scripts).
          { key: "Content-Security-Policy", value: "frame-ancestors 'none'; object-src 'none'; base-uri 'self'" },
          { key: "X-Frame-Options", value: "DENY" },
        ],
      },
    ];
  },
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${backendUrl}/api/:path*`,
      },
      {
        // MCP server (Streamable HTTP, bearer token only): lets `claude mcp add` use the public host.
        source: "/mcp",
        destination: `${backendUrl}/mcp`,
      },
      {
        source: "/healthz",
        destination: `${backendUrl}/healthz`,
      },
    ];
  },
};

export default nextConfig;
