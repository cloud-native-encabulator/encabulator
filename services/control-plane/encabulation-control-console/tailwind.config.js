/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        background: '#0a0a0a',
        surface: '#1a1a1a',
        primary: '#00d9ff',
        secondary: '#ff00ff',
        success: '#00ff88',
        warning: '#ffaa00',
        danger: '#ff0055',
        text: {
          primary: '#ffffff',
          secondary: '#a0a0a0',
          muted: '#666666',
        },
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', 'sans-serif'],
        mono: ['JetBrains Mono', 'monospace'],
      },
    },
  },
  plugins: [],
}
