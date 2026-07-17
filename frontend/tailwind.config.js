/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        primary: {
          DEFAULT: '#E89412',
          soft: '#FFE9A8',
          light: '#FFD65A',
          dark: '#2B1606',
        },
        background: {
          DEFAULT: '#FFF6D6',
          mist: '#FFE9A8',
          card: '#FFFDF6',
        },
        ink: {
          DEFAULT: '#2B1606',
          muted: '#6F4D24',
          soft: '#98723A',
        },
        line: '#E8CF86',
        success: '#1F9D62',
        violet: '#8B4A12',
      },
      borderRadius: {
        '2xl': '1rem',
        '3xl': '1.5rem',
      },
      keyframes: {
        'progress': {
          '0%': { transform: 'translateX(-100%)' },
          '100%': { transform: 'translateX(400%)' },
        },
        'fade-in': {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' },
        },
      },
      animation: {
        'progress': 'progress 1.5s ease-in-out infinite',
        'in': 'fade-in 0.2s ease-out',
      },
    },
  },
  plugins: [],
}
