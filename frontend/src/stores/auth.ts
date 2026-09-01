import { defineStore } from 'pinia'
import type { CurrentUser } from '@/types/api'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null as CurrentUser | null,
    checked: false,
  }),
  getters: {
    isAuthenticated: (state) => state.user !== null,
    isAdmin: (state) => state.user?.role === 'admin',
  },
  actions: {
    async fetchMe() {
      try {
        const res = await fetch('/api/v1/auth/me', { credentials: 'include' })
        this.user = res.ok ? ((await res.json()) as CurrentUser) : null
      } catch {
        this.user = null
      } finally {
        this.checked = true
      }
    },
    async logout() {
      await fetch('/api/v1/auth/logout', { method: 'POST', credentials: 'include' })
      this.user = null
    },
  },
})
