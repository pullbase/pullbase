import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  distDir: 'dist',
  trailingSlash: true,
  images: {
    unoptimized: true,
  },
  assetPrefix: process.env.NODE_ENV === 'production' ? '/ui' : '',
  basePath: '/ui',
};

export default nextConfig;
