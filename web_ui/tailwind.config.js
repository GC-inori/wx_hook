/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        primary: {
          DEFAULT: 'rgb(var(--primary-rgb, 7 193 96) / <alpha-value>)',
          50: 'rgb(var(--primary-rgb, 7 193 96) / 0.1)',
          100: 'rgb(var(--primary-rgb, 7 193 96) / 0.2)',
          200: 'rgb(var(--primary-rgb, 7 193 96) / 0.3)',
          300: 'rgb(var(--primary-rgb, 7 193 96) / 0.4)',
          400: 'rgb(var(--primary-rgb, 7 193 96) / 0.7)',
          500: 'rgb(var(--primary-rgb, 7 193 96) / 0.85)',
          600: 'rgb(var(--primary-rgb, 7 193 96) / 1)',
        },
        secondary: {
          DEFAULT: '#38bdf8',
          400: '#38bdf8',
          500: '#0284c7',
        },
        napcat: {
          bg: '#0a0d14',
          card: 'rgba(19, 23, 34, 0.75)',
          cardHover: 'rgba(24, 30, 46, 0.85)',
          border: 'rgba(255, 255, 255, 0.08)',
          purple: '#c084fc',
          cyan: '#38bdf8',
          emerald: '#07c160',
          amber: '#f59e0b',
        }
      },
      fontFamily: {
        mono: [
          'ui-monospace',
          'SFMono-Regular',
          'Menlo',
          'Monaco',
          'Consolas',
          '"Liberation Mono"',
          '"Courier New"',
          'monospace'
        ]
      },
      animation: {
        'nav-spin': 'navSpin 0.4s ease-out forwards',
        'fade-in': 'fadeIn 0.2s ease-out forwards',
      },
      keyframes: {
        navSpin: {
          '0%': { transform: 'scale(0.8)', opacity: '0.4' },
          '100%': { transform: 'scale(1)', opacity: '1' }
        },
        fadeIn: {
          '0%': { opacity: '0', transform: 'translateY(4px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        }
      }
    },
  },
  plugins: [],
}
