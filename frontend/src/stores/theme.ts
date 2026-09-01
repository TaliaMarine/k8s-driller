import { defineStore } from 'pinia'

const STORAGE_KEY = 'driller-theme'
type ThemeName = 'light' | 'dark'

function preferredTheme(): ThemeName {
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored === 'light' || stored === 'dark') return stored
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

export const useThemeStore = defineStore('theme', {
  state: () => ({
    name: preferredTheme() as ThemeName,
  }),
  actions: {
    toggle() {
      this.name = this.name === 'dark' ? 'light' : 'dark'
      localStorage.setItem(STORAGE_KEY, this.name)
    },
  },
})
