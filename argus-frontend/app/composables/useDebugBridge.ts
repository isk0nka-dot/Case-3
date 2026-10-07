// =============================================================================
// Argus AI — Debug Bridge (Reactive Singleton)
// =============================================================================
//
// Module-scoped reactive refs that allow the PerformanceDebugger (rendered at
// app root) to observe composable instances created deep in the component tree.
//
// Components set these refs when they create composable instances:
//   - useProctoringSession → registers resilience + healthGovernor
//   - PreExamCheck → registers visionEngine (if desired)
//
// The PerformanceDebugger reads these refs. When no session is active,
// all refs are null and the debugger shows "No active session".
//
// This avoids provide/inject (which only flows downward) and keeps
// modifications to existing composables minimal (1-2 lines each).
// =============================================================================

import { shallowRef } from 'vue'
import type { useHealthGovernor } from './useHealthGovernor'
import type { useResilience } from './useResilience'
import type { useVisionEngine } from './useVisionEngine'
import type { useProctoringSession } from './useProctoringSession'

// Module-scoped reactive refs — survive across component mounts
export const debugHealthGovernor = shallowRef<ReturnType<typeof useHealthGovernor> | null>(null)
export const debugResilience = shallowRef<ReturnType<typeof useResilience> | null>(null)
export const debugVisionEngine = shallowRef<ReturnType<typeof useVisionEngine> | null>(null)
export const debugSession = shallowRef<ReturnType<typeof useProctoringSession> | null>(null)

/**
 * Register a proctoring session and its sub-composables with the debug bridge.
 * Call this from useProctoringSession after creating the resilience layer.
 */
export function registerDebugSession(
  session: ReturnType<typeof useProctoringSession>,
  resilience: ReturnType<typeof useResilience> | null
): void {
  debugSession.value = session
  debugResilience.value = resilience
  if (resilience) {
    debugHealthGovernor.value = resilience.healthGovernor
  }
}

/**
 * Register a vision engine instance with the debug bridge.
 * Call this from PreExamCheck or wherever useVisionEngine is created.
 */
export function registerDebugVisionEngine(
  engine: ReturnType<typeof useVisionEngine>
): void {
  debugVisionEngine.value = engine
}

/**
 * Clear all debug registrations. Call on session stop.
 */
export function clearDebugSession(): void {
  debugSession.value = null
  debugResilience.value = null
  debugHealthGovernor.value = null
  debugVisionEngine.value = null
}
