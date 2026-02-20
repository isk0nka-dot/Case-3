// =============================================================================
// Argus AI — useFormatters Composable (Shared Date/Time Utilities)
// =============================================================================
//
// Consolidates date/time formatting functions that were duplicated across 12+
// files with inconsistent implementations (some used 'ru-RU', some 'en-US').
//
// All formatters use 'ru-RU' locale for consistency across the application.
//
// Usage:
//   const { formatDate, formatTime, formatDateTime, formatVideoTimestamp, formatTimeAgo } = useFormatters()
//
// =============================================================================

// ---------------------------------------------------------------------------
// Cached formatters (avoid re-creating Intl objects every call)
// ---------------------------------------------------------------------------

const dateFormatter = new Intl.DateTimeFormat('ru-RU', {
  day: '2-digit',
  month: 'long',
  year: 'numeric'
})

const timeFormatter = new Intl.DateTimeFormat('ru-RU', {
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit'
})

const dateTimeFormatter = new Intl.DateTimeFormat('ru-RU', {
  day: '2-digit',
  month: 'long',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit'
})

const dateTimeFullFormatter = new Intl.DateTimeFormat('ru-RU', {
  day: '2-digit',
  month: 'long',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit'
})

const shortTimeFormatter = new Intl.DateTimeFormat('ru-RU', {
  hour: '2-digit',
  minute: '2-digit'
})

// ---------------------------------------------------------------------------
// Pure Utility Functions (usable without composable wrapper)
// ---------------------------------------------------------------------------

/**
 * Format an ISO date string as "12 января 2025".
 */
export function formatDate(iso: string): string {
  if (!iso) return '—'
  try {
    return dateFormatter.format(new Date(iso))
  } catch {
    return '—'
  }
}

/**
 * Format an ISO date string as "12:34:56".
 */
export function formatTime(iso: string): string {
  if (!iso) return '—'
  try {
    return timeFormatter.format(new Date(iso))
  } catch {
    return '—'
  }
}

/**
 * Format an ISO date string as "12 января 2025, 12:34".
 */
export function formatDateTime(iso: string): string {
  if (!iso) return '—'
  try {
    return dateTimeFormatter.format(new Date(iso))
  } catch {
    return '—'
  }
}

/**
 * Format an ISO date string as "12 января 2025, 12:34:56" (with seconds).
 */
export function formatDateTimeFull(iso: string): string {
  if (!iso) return '—'
  try {
    return dateTimeFullFormatter.format(new Date(iso))
  } catch {
    return '—'
  }
}

/**
 * Format an ISO date string as "12:34" (short time).
 */
export function formatTimeShort(iso: string): string {
  if (!iso) return '—'
  try {
    return shortTimeFormatter.format(new Date(iso))
  } catch {
    return '—'
  }
}

/**
 * Format seconds as "H:MM:SS" or "M:SS" video timestamp.
 */
export function formatVideoTimestamp(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  return `${m}:${String(s).padStart(2, '0')}`
}

/**
 * Format a date string as relative time ("5 минут назад", "2 часа назад").
 */
export function formatTimeAgo(iso: string): string {
  if (!iso) return '—'
  try {
    const now = Date.now()
    const then = new Date(iso).getTime()
    const diffSec = Math.floor((now - then) / 1000)

    if (diffSec < 10) return 'только что'
    if (diffSec < 60) return `${diffSec} сек. назад`
    if (diffSec < 3600) return `${Math.floor(diffSec / 60)} мин. назад`
    if (diffSec < 86400) return `${Math.floor(diffSec / 3600)} ч. назад`
    return `${Math.floor(diffSec / 86400)} дн. назад`
  } catch {
    return '—'
  }
}

/**
 * Format an ISO string as time for event log display ("12:34:56").
 * Alias for formatTime — used for semantic clarity in monitoring contexts.
 */
export function formatStartTime(iso: string): string {
  return formatTime(iso)
}

// ---------------------------------------------------------------------------
// Composable Wrapper (for use in Vue components via auto-import)
// ---------------------------------------------------------------------------

/**
 * Shared formatting utilities composable.
 *
 * Returns all date/time/number formatters.
 * Replaces 23+ duplicate formatter definitions across the codebase.
 *
 * All formatters are also exported as standalone functions for use outside
 * of Vue component context (e.g., in stores, utilities, or tests).
 */
export function useFormatters() {
  return {
    formatDate,
    formatTime,
    formatDateTime,
    formatDateTimeFull,
    formatTimeShort,
    formatVideoTimestamp,
    formatTimeAgo,
    formatStartTime
  }
}
