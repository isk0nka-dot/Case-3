// =============================================================================
// useArgusI18n — dependency-free reactive i18n for the Argus dashboard.
// =============================================================================
// Named to avoid colliding with Nuxt UI's own `useLocale`. Provides a shared
// reactive locale (Nuxt useState) + t(key) lookup against the catalogs in
// ~/i18n/messages. Russian is the fallback. Persists to localStorage and keeps
// <html lang> in sync. See app/plugins/i18n.client.ts for restore.
// =============================================================================

import { messages, DEFAULT_LOCALE, type Locale } from '~/i18n/messages'

export const LOCALE_STORAGE_KEY = 'argus_locale'

export function useArgusI18n() {
  const locale = useState<Locale>('argus-locale', () => DEFAULT_LOCALE)

  function setLocale(next: Locale): void {
    if (!messages[next]) return
    locale.value = next
    if (import.meta.client) {
      try {
        localStorage.setItem(LOCALE_STORAGE_KEY, next)
      } catch {
        /* storage unavailable — non-fatal */
      }
      document.documentElement.lang = next
    }
  }

  /** Translate a dotted key; falls back to Russian, then the raw key. */
  function t(key: string): string {
    return messages[locale.value]?.[key] ?? messages[DEFAULT_LOCALE][key] ?? key
  }

  return { locale, setLocale, t }
}
