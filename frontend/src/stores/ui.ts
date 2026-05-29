import { defineStore } from 'pinia'
import { detectInitialLocale, normalizeLocale, type SupportedLocale } from '../i18n'

export const useUiStore = defineStore('ui', {
  state: () => ({
    showSettings: false,
    showHelp: false,
    locale: detectInitialLocale() as SupportedLocale,
    sidebarCollapsed: false,
  }),
  actions: {
    setLocale(value: string) {
      const next = normalizeLocale(value)
      this.locale = next
      if (typeof window !== 'undefined') {
        window.localStorage.setItem('ui.locale', next)
      }
    },
    toggleSidebar() {
      this.sidebarCollapsed = !this.sidebarCollapsed
    },
  },
})
