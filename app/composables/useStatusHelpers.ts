// =============================================================================
// Argus AI — useStatusHelpers Composable (Shared Status & Severity Utilities)
// =============================================================================
//
// Consolidates status/severity label, color, icon, and background mappers
// that were duplicated across 8+ files.
//
// Covers three semantic domains:
//   1. Session Status:  reviewed | pending | voided
//   2. Appeal Status:   submitted | under_review | upheld | overturned | withdrawn
//   3. Event Severity:  critical | warning | info
//
// Usage:
//   const { sessionStatusLabel, appealStatusColor, severityColor } = useStatusHelpers()
//
// =============================================================================

import { type ColorFn, useColors } from './useColors'

// ---------------------------------------------------------------------------
// Session Status Helpers (archive, dashboard)
// ---------------------------------------------------------------------------

export type ArchiveSessionStatus = 'reviewed' | 'pending' | 'voided'

export function sessionStatusLabel(status: string): string {
  switch (status) {
    case 'reviewed': return 'Проверено'
    case 'pending': return 'На проверке'
    case 'voided': return 'Аннулировано'
    default: return '—'
  }
}

export function sessionStatusColor(status: string): string {
  switch (status) {
    case 'reviewed': return 'var(--argus-success)'
    case 'pending': return 'var(--argus-warning)'
    case 'voided': return 'var(--argus-error)'
    default: return 'var(--argus-text-dimmed)'
  }
}

export function sessionStatusIcon(status: string): string {
  switch (status) {
    case 'reviewed': return 'i-lucide-check-circle'
    case 'pending': return 'i-lucide-clock'
    case 'voided': return 'i-lucide-x-circle'
    default: return 'i-lucide-circle'
  }
}

export function sessionStatusBg(
  status: string,
  opacity: number,
  colors: { successBg: ColorFn; warningBg: ColorFn; errorBg: ColorFn }
): string {
  switch (status) {
    case 'reviewed': return colors.successBg(opacity)
    case 'pending': return colors.warningBg(opacity)
    case 'voided': return colors.errorBg(opacity)
    default: return 'transparent'
  }
}

// ---------------------------------------------------------------------------
// Appeal Status Helpers
// ---------------------------------------------------------------------------

export type AppealStatus = 'submitted' | 'under_review' | 'upheld' | 'overturned' | 'withdrawn'

export function appealStatusLabel(status: string): string {
  switch (status) {
    case 'submitted': return 'Подана'
    case 'under_review': return 'На рассмотрении'
    case 'upheld': return 'Удовлетворена'
    case 'overturned': return 'Отклонена'
    case 'withdrawn': return 'Отозвана'
    default: return status
  }
}

export function appealStatusColor(status: string): string {
  switch (status) {
    case 'submitted': return 'var(--argus-accent)'
    case 'under_review': return 'var(--argus-warning)'
    case 'upheld': return 'var(--argus-success)'
    case 'overturned': return 'var(--argus-error)'
    case 'withdrawn': return 'var(--argus-text-dimmed)'
    default: return 'var(--argus-text-dimmed)'
  }
}

export function appealStatusIcon(status: string): string {
  switch (status) {
    case 'submitted': return 'i-lucide-inbox'
    case 'under_review': return 'i-lucide-search'
    case 'upheld': return 'i-lucide-check-circle'
    case 'overturned': return 'i-lucide-x-circle'
    case 'withdrawn': return 'i-lucide-archive'
    default: return 'i-lucide-circle'
  }
}

export function appealStatusBg(
  status: string,
  opacity: number,
  colors: { accentBg: ColorFn; warningBg: ColorFn; successBg: ColorFn; errorBg: ColorFn; infoBg: ColorFn }
): string {
  switch (status) {
    case 'submitted': return colors.accentBg(opacity)
    case 'under_review': return colors.warningBg(opacity)
    case 'upheld': return colors.successBg(opacity)
    case 'overturned': return colors.errorBg(opacity)
    case 'withdrawn': return colors.infoBg(opacity)
    default: return 'transparent'
  }
}

export function isAppealTerminal(status: string): boolean {
  return ['upheld', 'overturned', 'withdrawn'].includes(status)
}

// ---------------------------------------------------------------------------
// Event Severity Helpers
// ---------------------------------------------------------------------------

export type EventSeverity = 'critical' | 'warning' | 'info'

export function severityColor(severity: string): string {
  switch (severity) {
    case 'critical': return 'var(--argus-error)'
    case 'warning': return 'var(--argus-warning)'
    case 'info': return 'var(--argus-text-dimmed)'
    default: return 'var(--argus-text-dimmed)'
  }
}

export function severityBg(
  severity: string,
  opacity: number,
  colors: { errorBg: ColorFn; warningBg: ColorFn; infoBg: ColorFn }
): string {
  switch (severity) {
    case 'critical': return colors.errorBg(opacity)
    case 'warning': return colors.warningBg(opacity)
    case 'info': return colors.infoBg(opacity)
    default: return 'transparent'
  }
}

export function severityLabel(severity: string): string {
  switch (severity) {
    case 'critical': return 'КРИТ'
    case 'warning': return 'ВНИМАНИЕ'
    case 'info': return 'ИНФО'
    default: return severity
  }
}

// ---------------------------------------------------------------------------
// Event Type Helpers (event icons, source labels)
// ---------------------------------------------------------------------------

export function eventIcon(type: string): string {
  switch (type) {
    case 'phone_detected': return 'i-lucide-smartphone'
    case 'phone_in_hand': return 'i-lucide-hand'
    case 'objects_on_desk': return 'i-lucide-package-search'
    case 'out_of_frame_side': return 'i-lucide-scan-line'
    case 'book_detected': return 'i-lucide-book-open'
    case 'gaze_deviation': return 'i-lucide-eye-off'
    case 'gaze_telemetry': return 'i-lucide-eye'
    case 'face_mismatch': return 'i-lucide-user-x'
    case 'face_not_detected': return 'i-lucide-user-x'
    case 'face_spoof_detected': return 'i-lucide-shield-alert'
    case 'tab_switch': return 'i-lucide-app-window'
    case 'audio_anomaly': return 'i-lucide-mic-off'
    case 'earbuds_detected': return 'i-lucide-headphones'
    case 'multiple_persons': return 'i-lucide-users'
    // AI Vision events
    case 'head_pose_anomaly': return 'i-lucide-rotate-3d'
    case 'head_pose_telemetry': return 'i-lucide-rotate-3d'
    case 'liveness_check_failed': return 'i-lucide-scan-face'
    case 'face_occluded': return 'i-lucide-eye-off'
    case 'face_embedding_telemetry': return 'i-lucide-fingerprint'
    // AI Audio events
    case 'whisper_detected': return 'i-lucide-ear'
    case 'second_speaker_detected': return 'i-lucide-users'
    case 'audio_playback_detected': return 'i-lucide-volume-2'
    case 'audio_level_telemetry': return 'i-lucide-activity'
    // Behavioral Analysis (Kernel-Level)
    case 'typing_dynamics': return 'i-lucide-keyboard'
    case 'typing_anomaly': return 'i-lucide-keyboard'
    case 'cursor_sync': return 'i-lucide-mouse-pointer'
    case 'hand_cursor_desync': return 'i-lucide-mouse-pointer'
    case 'mouse_telemetry': return 'i-lucide-mouse-pointer-2'
    case 'focus_score_update': return 'i-lucide-brain'
    default: return 'i-lucide-alert-triangle'
  }
}

export function isBehavioralEvent(type: string): boolean {
  return ['typing_dynamics', 'typing_anomaly', 'cursor_sync', 'hand_cursor_desync', 'mouse_telemetry', 'focus_score_update'].includes(type)
}

export function isAudioEvent(type: string): boolean {
  return ['audio_anomaly', 'background_voices', 'whispering', 'whisper_detected', 'second_speaker_detected', 'audio_playback_detected', 'audio_level_telemetry', 'voice_activity', 'smart_noise_classified'].includes(type)
}

export function isVisionAIEvent(type: string): boolean {
  return ['head_pose_anomaly', 'liveness_check_failed', 'face_occluded', 'head_pose_telemetry', 'face_embedding_telemetry'].includes(type)
}

// ---------------------------------------------------------------------------
// Source Helpers (camera source labels)
// ---------------------------------------------------------------------------

export function sourceLabel(source: string): string {
  switch (source) {
    case 'webcam': return 'Веб-камера'
    case 'side': return 'Боковая камера'
    case 'system': return 'Система'
    default: return '—'
  }
}

export function sourceIcon(source: string): string {
  switch (source) {
    case 'webcam': return 'i-lucide-video'
    case 'side': return 'i-lucide-camera'
    case 'system': return 'i-lucide-monitor'
    default: return 'i-lucide-circle'
  }
}

export function sourceColor(source: string, isDark: boolean): string {
  switch (source) {
    case 'webcam': return 'var(--argus-accent)'
    case 'side': return isDark ? '#a78bfa' : '#7c3aed'
    case 'system': return 'var(--argus-text-dimmed)'
    default: return 'var(--argus-text-dimmed)'
  }
}

export function isSideEvent(source: string): boolean {
  return source === 'side'
}

// ---------------------------------------------------------------------------
// Integrity Score Helpers
// ---------------------------------------------------------------------------

export function integrityColor(score: number): string {
  if (score < 50) return 'var(--argus-error)'
  if (score < 70) return 'var(--argus-warning)'
  return 'var(--argus-success)'
}

export function integrityGradient(score: number, isDark: boolean): string {
  if (score >= 80) return isDark ? 'linear-gradient(90deg, #34D399, #10B981)' : 'linear-gradient(90deg, #10A34A, #0D8A3E)'
  if (score >= 60) return isDark ? 'linear-gradient(90deg, #FBBF24, #F59E0B)' : 'linear-gradient(90deg, #E67E22, #C96E1A)'
  return isDark ? 'linear-gradient(90deg, #F87171, #EF4444)' : 'linear-gradient(90deg, #E03E3E, #C92B2B)'
}

// ---------------------------------------------------------------------------
// Export Status Helpers
// ---------------------------------------------------------------------------

export type ExportStatus = 'pending' | 'processing' | 'completed' | 'failed' | 'expired'

export function exportStatusLabel(status: string): string {
  switch (status) {
    case 'pending': return 'В очереди'
    case 'processing': return 'Экспорт...'
    case 'completed': return 'Готов'
    case 'failed': return 'Ошибка'
    case 'expired': return 'Истёк'
    default: return status
  }
}

export function exportStatusColor(status: string): string {
  switch (status) {
    case 'pending': return 'var(--argus-warning)'
    case 'processing': return 'var(--argus-accent)'
    case 'completed': return 'var(--argus-success)'
    case 'failed': return 'var(--argus-error)'
    case 'expired': return 'var(--argus-text-dimmed)'
    default: return 'var(--argus-text-dimmed)'
  }
}

export function exportStatusIcon(status: string): string {
  switch (status) {
    case 'pending': return 'i-lucide-clock'
    case 'processing': return 'i-lucide-loader'
    case 'completed': return 'i-lucide-check-circle'
    case 'failed': return 'i-lucide-x-circle'
    case 'expired': return 'i-lucide-timer-off'
    default: return 'i-lucide-circle'
  }
}

// ---------------------------------------------------------------------------
// Composable Wrapper
// ---------------------------------------------------------------------------

/**
 * Shared status & severity utility composable.
 *
 * Returns all label/color/icon mappers for session status, appeal status,
 * event severity, event types, and source labels.
 *
 * Replaces 25+ duplicate mapper definitions across the codebase.
 */
export function useStatusHelpers() {
  const colors = useColors()

  return {
    // Session status
    sessionStatusLabel,
    sessionStatusColor,
    sessionStatusIcon,
    sessionStatusBg: (status: string, opacity: number) => sessionStatusBg(status, opacity, colors),

    // Appeal status
    appealStatusLabel,
    appealStatusColor,
    appealStatusIcon,
    appealStatusBg: (status: string, opacity: number) => appealStatusBg(status, opacity, colors),
    isAppealTerminal,

    // Severity
    severityColor,
    severityBg: (severity: string, opacity: number) => severityBg(severity, opacity, colors),
    severityLabel,

    // Event types
    eventIcon,
    isBehavioralEvent,
    isAudioEvent,
    isVisionAIEvent,

    // Source
    sourceLabel,
    sourceIcon,
    sourceColor: (source: string) => sourceColor(source, colors.isDark.value),
    isSideEvent,

    // Integrity
    integrityColor,
    integrityGradient: (score: number) => integrityGradient(score, colors.isDark.value),

    // Export status
    exportStatusLabel,
    exportStatusColor,
    exportStatusIcon
  }
}
