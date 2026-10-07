// =============================================================================
// Argus AI — useColors Composable (Shared Color Utilities)
// =============================================================================
//
// Consolidates color helper functions that were duplicated across 16+ files.
// Provides theme-aware RGBA color generators for consistent UI styling.
//
// Usage:
//   const { accentBg, errorBg, successBg, warningBg, purpleBg, isDark } = useColors()
//   // In templates: :style="{ background: accentBg(0.1) }"
//
// =============================================================================

import { computed, type ComputedRef } from 'vue'
import { useColorMode } from '#imports'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** A function that returns a theme-aware RGBA color string. */
export type ColorFn = (opacity: number) => string

/** All color helpers returned by useColors. */
export interface ColorsAPI {
  isDark: ComputedRef<boolean>
  accentBg: ColorFn
  errorBg: ColorFn
  successBg: ColorFn
  warningBg: ColorFn
  purpleBg: ColorFn
  infoBg: ColorFn
}

// ---------------------------------------------------------------------------
// Color RGBA Pairs: [dark, light] per semantic role
// ---------------------------------------------------------------------------

const COLOR_PAIRS = {
  accent: { dark: '56, 189, 248', light: '37, 99, 235' },
  error: { dark: '248, 113, 113', light: '224, 62, 62' },
  success: { dark: '52, 211, 153', light: '16, 163, 74' },
  warning: { dark: '251, 191, 36', light: '230, 126, 34' },
  purple: { dark: '167, 139, 250', light: '139, 92, 246' },
  info: { dark: '100, 116, 139', light: '148, 163, 184' }
} as const

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

/**
 * Shared color utility composable.
 *
 * Returns theme-aware color functions that generate `rgba(...)` strings.
 * Each function accepts an `opacity` parameter (0-1) and returns the
 * appropriate color for the current dark/light mode.
 *
 * Replaces 64+ duplicate function definitions across the codebase.
 */
export function useColors(): ColorsAPI {
  const colorMode = useColorMode()
  const isDark = computed(() => colorMode.value === 'dark')

  function makeColorFn(role: keyof typeof COLOR_PAIRS): ColorFn {
    return (opacity: number): string => {
      const rgb = isDark.value ? COLOR_PAIRS[role].dark : COLOR_PAIRS[role].light
      return `rgba(${rgb}, ${opacity})`
    }
  }

  return {
    isDark,
    accentBg: makeColorFn('accent'),
    errorBg: makeColorFn('error'),
    successBg: makeColorFn('success'),
    warningBg: makeColorFn('warning'),
    purpleBg: makeColorFn('purple'),
    infoBg: makeColorFn('info')
  }
}
