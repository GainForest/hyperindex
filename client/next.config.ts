import type { NextConfig } from "next";

function normalizeUrl(url: string): string {
  const trimmed = url.trim().replace(/\/+$/, "");
  if (!trimmed) return "";
  if (trimmed.startsWith("http://") || trimmed.startsWith("https://")) return trimmed;
  return `https://${trimmed}`;
}

const nextConfig: NextConfig = {
  // Enable standalone output for Docker deployment
  output: "standalone",
  // Required so Proxy can emit absolute localhost -> 127.0.0.1 redirect URLs without Next normalizing the Location header to `/`.
  skipProxyUrlNormalize: true,
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
  // Proxy API requests to Hyperindex backend during development
  async rewrites() {
    const apiUrl =
      normalizeUrl(process.env.NEXT_PUBLIC_HYPERINDEX_URL || "") ||
      "http://localhost:8080";
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
