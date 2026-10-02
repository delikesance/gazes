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
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${backendUrl}/api/:path*`,
      },
      {
        source: "/healthz",
        destination: `${backendUrl}/healthz`,
      },
    ];
  },
};

export default nextConfig;
