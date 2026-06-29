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
          DEFAULT: '#A65F1A',
          light: '#D49A36',
          dark: '#3A2414',
        },
        background: {
          DEFAULT: '#F4F5F1',
          mist: '#ECEFE8',
          card: '#FEFCF7',
        },
        ink: {
          DEFAULT: '#1E2420',
          muted: '#586158',
          soft: '#7A8278',
        },
        line: '#D9D7CB',
        success: '#22C55E',
        violet: '#38546E',
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
