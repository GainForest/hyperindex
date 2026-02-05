import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Allow external images from Bluesky CDN
  images: {
    remotePatterns: [
      {
        protocol: "https",
        hostname: "cdn.bsky.app",
        pathname: "/img/**",
      },
    ],
  },
  // Proxy API requests to Hypergoat backend during development
  async rewrites() {
    const apiUrl = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
    return [
      {
        source: "/admin/graphql",
        destination: `${apiUrl}/admin/graphql`,
      },
      {
        source: "/graphql",
        destination: `${apiUrl}/graphql`,
      },
      {
        source: "/graphiql",
        destination: `${apiUrl}/graphiql`,
      },
      {
        source: "/oauth/:path*",
        destination: `${apiUrl}/oauth/:path*`,
      },
      {
        source: "/.well-known/:path*",
        destination: `${apiUrl}/.well-known/:path*`,
      },
    ];
  },
};

export default nextConfig;
