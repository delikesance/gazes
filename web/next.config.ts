import type { NextConfig } from "next";

const backendUrl = process.env.BACKEND_URL || "http://127.0.0.1:8090";

const nextConfig: NextConfig = {
  output: "standalone",
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
