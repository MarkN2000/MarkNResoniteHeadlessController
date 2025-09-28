/** @type {import('tailwindcss').Config} */
import { skeleton } from '@skeletonlabs/tw-plugin';

export default {
  content: ['./src/**/*.{html,svelte,ts}'],
  theme: {
    extend: {
      colors: {
        resonite: {
          dark: '#11151d',
          mid: '#2b2f35',
          light: '#e1e1e0',
          yellow: '#f8f770',
          green: '#59eb5c',
          red: '#ff7676',
          purple: '#ba64f2',
          cyan: '#61d1fa',
          orange: '#e69e50'
        }
      }
    }
  },
  plugins: [
    skeleton({
      themes: {
        preset: [
          {
            name: 'resonite',
            enhancements: true,
            properties: {
              '--theme-font-family-base': '"Noto Sans JP", system-ui, sans-serif',
              '--theme-font-color-base': 'rgb(var(--color-surface-900))',
              '--theme-font-color-dark': 'rgb(var(--color-surface-50))',
              '--theme-rounded-base': '0.75rem',
              '--theme-rounded-container': '1rem',
              '--color-primary-500': '#f8f770',
              '--color-secondary-500': '#61d1fa',
              '--color-tertiary-500': '#ba64f2',
              '--color-success-500': '#59eb5c',
              '--color-warning-500': '#e69e50',
              '--color-error-500': '#ff7676',
              '--color-surface-100': '#e1e1e0',
              '--color-surface-200': '#86888b',
              '--color-surface-300': '#4a4a4d',
              '--color-surface-400': '#2b2f35',
              '--color-surface-500': '#11151d',
              '--color-surface-600': '#11151d',
              '--color-surface-700': '#11151d',
              '--color-surface-800': '#11151d',
              '--color-surface-900': '#11151d'
            }
          }
        ]
      }
    }),
    require('@tailwindcss/forms')
  ]
};

