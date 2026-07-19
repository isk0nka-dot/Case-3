// Restore the saved locale on client boot and set <html lang> accordingly.
import { messages, DEFAULT_LOCALE, type Locale } from '~/i18n/messages'

export default defineNuxtPlugin(() => {
  const { setLocale } = useArgusI18n()
  let saved: string | null = null
  try {
    saved = localStorage.getItem(LOCALE_STORAGE_KEY)
  } catch {
    /* storage unavailable — non-fatal */
  }
  if (saved && messages[saved as Locale]) {
    setLocale(saved as Locale)
  } else {
    document.documentElement.lang = DEFAULT_LOCALE
  }
})
