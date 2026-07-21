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
          DEFAULT: 'rgb(var(--color-primary) / <alpha-value>)',
          soft: 'rgb(var(--color-primary-soft) / <alpha-value>)',
          light: 'rgb(var(--color-primary-light) / <alpha-value>)',
          dark: 'rgb(var(--color-primary-dark) / <alpha-value>)',
        },
        'on-primary': 'rgb(var(--color-on-primary) / <alpha-value>)',
        'on-primary-dark': 'rgb(var(--color-on-primary-dark) / <alpha-value>)',
        'brand-panel': 'rgb(var(--color-brand-panel) / <alpha-value>)',
        'on-brand-panel': 'rgb(var(--color-on-brand-panel) / <alpha-value>)',
        background: {
          DEFAULT: 'rgb(var(--color-background) / <alpha-value>)',
          mist: 'rgb(var(--color-background-mist) / <alpha-value>)',
          card: 'rgb(var(--color-background-card) / <alpha-value>)',
        },
        ink: {
          DEFAULT: 'rgb(var(--color-ink) / <alpha-value>)',
          muted: 'rgb(var(--color-ink-muted) / <alpha-value>)',
          soft: 'rgb(var(--color-ink-soft) / <alpha-value>)',
        },
        line: 'rgb(var(--color-line) / <alpha-value>)',
        success: 'rgb(var(--color-success) / <alpha-value>)',
        'on-success': 'rgb(var(--color-on-success) / <alpha-value>)',
        'success-soft': 'rgb(var(--color-success-soft) / <alpha-value>)',
        'success-muted': 'rgb(var(--color-success-muted) / <alpha-value>)',
        'success-line': 'rgb(var(--color-success-line) / <alpha-value>)',
        'success-ink': 'rgb(var(--color-success-ink) / <alpha-value>)',
        danger: 'rgb(var(--color-danger) / <alpha-value>)',
        'on-danger': 'rgb(var(--color-on-danger) / <alpha-value>)',
        'danger-soft': 'rgb(var(--color-danger-soft) / <alpha-value>)',
        'danger-muted': 'rgb(var(--color-danger-muted) / <alpha-value>)',
        'danger-line': 'rgb(var(--color-danger-line) / <alpha-value>)',
        'danger-ink': 'rgb(var(--color-danger-ink) / <alpha-value>)',
        warning: 'rgb(var(--color-warning) / <alpha-value>)',
        'on-warning': 'rgb(var(--color-on-warning) / <alpha-value>)',
        'warning-soft': 'rgb(var(--color-warning-soft) / <alpha-value>)',
        'warning-muted': 'rgb(var(--color-warning-muted) / <alpha-value>)',
        'warning-line': 'rgb(var(--color-warning-line) / <alpha-value>)',
        'warning-ink': 'rgb(var(--color-warning-ink) / <alpha-value>)',
        neutral: 'rgb(var(--color-neutral) / <alpha-value>)',
        'on-neutral': 'rgb(var(--color-on-neutral) / <alpha-value>)',
        'neutral-soft': 'rgb(var(--color-neutral-soft) / <alpha-value>)',
        'neutral-muted': 'rgb(var(--color-neutral-muted) / <alpha-value>)',
        'neutral-line': 'rgb(var(--color-neutral-line) / <alpha-value>)',
        'neutral-ink': 'rgb(var(--color-neutral-ink) / <alpha-value>)',
        violet: 'rgb(var(--color-primary-dark) / <alpha-value>)',
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
