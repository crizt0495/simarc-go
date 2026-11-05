import type { Config } from 'tailwindcss';

const config: Config = {
  darkMode: 'class',
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        app: 'var(--c-app-bg)',
        sidebar: 'var(--c-sidebar-bg)',
        surface: 'var(--c-surface)',
        'surface-2': 'var(--c-surface-2)',
        primary: {
          DEFAULT: 'var(--c-primary)',
          hover: 'var(--c-primary-hover)',
          soft: 'var(--c-primary-soft)',
        },
        ink: {
          1: 'var(--c-text-1)',
          2: 'var(--c-text-2)',
          3: 'var(--c-text-3)',
        },
        line: {
          DEFAULT: 'var(--c-border)',
          soft: 'var(--c-border-soft)',
        },
        ok: 'var(--c-success)',
        warn: 'var(--c-warning)',
        bad: 'var(--c-danger)',
        info: 'var(--c-info)',
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', 'sans-serif'],
      },
      fontSize: {
        'page-title': ['24px', '32px'],
        'section': ['16px', '24px'],
        'card-title': ['14px', '20px'],
        'body': ['13px', '18px'],
        'label': ['12px', '16px'],
        'caption': ['11px', '16px'],
        'btn': ['13px', '20px'],
      },
      spacing: {
        'topbar': '64px',
      },
      boxShadow: {
        soft: '0 1px 2px rgba(16, 24, 40, 0.06)',
        modal: '0 8px 24px rgba(16, 24, 40, 0.12)',
      },
      animation: {
        'fade-in': 'fadeIn 180ms ease-out',
        'toast-in': 'toastIn 200ms ease-out',
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0', transform: 'translateY(4px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
        toastIn: {
          '0%': { opacity: '0', transform: 'translateX(16px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' },
        },
      },
      transitionDuration: {
        DEFAULT: '150ms',
        200: '200ms',
      },
    },
  },
  plugins: [],
};

export default config;
