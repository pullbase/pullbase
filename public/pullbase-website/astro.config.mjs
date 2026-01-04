// @ts-check
import { defineConfig } from 'astro/config';
import tailwind from '@astrojs/tailwind';

// https://astro.build/config
export default defineConfig({
  integrations: [tailwind()],
  site: 'https://pullbase.io',
  build: {
    inlineStylesheets: 'always',
    assets: '_astro',
  },
  vite: {
    build: {
      minify: 'terser',
      cssMinify: 'esbuild',
      sourcemap: false,
      terserOptions: {
        compress: {
          drop_console: true,
          drop_debugger: true,
          pure_funcs: ['console.log', 'console.info', 'console.warn'],
          passes: 3,
          unsafe: true,
          unsafe_comps: true,
          unsafe_math: true,
          hoist_funs: true,
          hoist_vars: true,
          reduce_vars: true,
          toplevel: true,
        },
        mangle: {
          safari10: true,
          toplevel: true,
        },
        format: {
          comments: false,
        },
      },
      rollupOptions: {
        output: {
          manualChunks: (id) => {
            if (id.includes('node_modules')) return 'vendor';
            if (id.includes('astro')) return 'astro';
          },
          assetFileNames: (assetInfo) => {
            if (!assetInfo.names?.length) return 'assets/[name]-[hash][extname]';
            
            const name = assetInfo.names[0];
            const info = name.split('.');
            const extType = info[info.length - 1];
            
            if (/\.(png|jpe?g|svg|gif|tiff|bmp|ico)$/i.test(name)) {
              return `images/[name]-[hash][extname]`;
            } else if (/\.(css)$/i.test(name)) {
              return `css/[name]-[hash][extname]`;
            }
            return `assets/[name]-[hash][extname]`;
          },
          chunkFileNames: 'js/[name]-[hash].js',
          entryFileNames: 'js/[name]-[hash].js',
        },
      },
    },
  },
  compressHTML: true,
});
