import 'vuetify/styles'
import '@mdi/font/css/materialdesignicons.css'
import { createVuetify } from 'vuetify'

/**
 * One severity scale used everywhere a pressure state appears — node card
 * border, pod chip, progress-bar fill (SPECS.md §7.2). `wildwest` is kept
 * visually distinct from the OOM/Throttling severity colors so
 * "misconfigured" and "under pressure" are never confused at a glance.
 */
const sharedColors = {
  healthy: '#2E7D32',
  watch: '#0288D1',
  warning: '#F9A825',
  critical: '#C62828',
  wildwest: '#FF6D00',
}

export default createVuetify({
  theme: {
    defaultTheme: 'dark',
    themes: {
      light: {
        dark: false,
        colors: {
          primary: '#1867C0',
          background: '#F5F5F7',
          surface: '#FFFFFF',
          ...sharedColors,
        },
      },
      dark: {
        dark: true,
        colors: {
          primary: '#42A5F5',
          background: '#121212',
          surface: '#1E1E1E',
          ...sharedColors,
        },
      },
    },
  },
})
