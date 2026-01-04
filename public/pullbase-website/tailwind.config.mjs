/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/**/*.{astro,html,js,jsx,md,mdx,svelte,ts,tsx,vue}'],
  darkMode: 'class',
  theme: {
    extend: {
      backgroundImage: {
        'technical-grid':
          'linear-gradient(to right, #1a1a1a 1px, transparent 1px), linear-gradient(to bottom, #1a1a1a 1px, transparent 1px)',
      },
      animation: {
        'marquee-ltr': 'marquee-ltr 25s linear infinite',
      },
      keyframes: {
        'marquee-ltr': {
          '0%': { transform: 'translateX(-100%)' },
          '100%': { transform: 'translateX(0%)' },
        },
      },
      colors: {
        primary: {
          50: '#f0f7ff',
          100: '#e0effe',
          200: '#bae0fd',
          300: '#7cc8fb',
          400: '#36aaf5',
          500: '#0c8ee4',
          600: '#0070c2',
          700: '#005a9e',
          800: '#004c83',
          900: '#00406d',
          950: '#1C2D4C',
        },
        studio: {
          black: '#050505',
          panel: '#0A0A0A',
          border: '#222222',
          accent: '#3b82f6',
          text: '#EDEDED',
          muted: '#888888',
        },
      },
      fontFamily: {
        sans: ['Space Grotesk', 'sans-serif'],
        body: ['Inter', 'sans-serif'],
        mono: ['JetBrains Mono', 'monospace'],
      },
    },
  },
  plugins: [],
};
