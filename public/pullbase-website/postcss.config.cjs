module.exports = {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
    ...(process.env.NODE_ENV === 'production' ? {
      cssnano: {
        preset: ['default', {
          discardComments: {
            removeAll: true,
          },
          mergeRules: true,
          minifySelectors: true,
          normalizeWhitespace: true,
        }],
      },
    } : {}),
  },
};
