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
  colors: { successBg: ColorFn, warningBg: ColorFn, errorBg: ColorFn }
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
  colors: { accentBg: ColorFn, warningBg: ColorFn, successBg: ColorFn, errorBg: ColorFn, infoBg: ColorFn }
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
// Review Decision Helpers (proctor review panel)
// ---------------------------------------------------------------------------

export type ReviewDecisionType = 'confirmed' | 'dismissed' | 'escalated'

export function reviewDecisionLabel(d: string): string {
  switch (d) {
    case 'confirmed': return 'Подтверждено'
    case 'dismissed': return 'Отклонено'
    case 'escalated': return 'Эскалировано'
    default: return d
  }
}

export function reviewDecisionColor(d: string): string {
  switch (d) {
    case 'confirmed': return 'var(--argus-error)'
    case 'dismissed': return 'var(--argus-success)'
    case 'escalated': return 'var(--argus-warning)'
    default: return 'var(--argus-text-dimmed)'
  }
}

export function reviewDecisionBg(d: string, isDark: boolean): string {
  switch (d) {
    case 'confirmed': return isDark ? 'rgba(248, 113, 113, 0.1)' : 'rgba(224, 62, 62, 0.08)'
    case 'dismissed': return isDark ? 'rgba(52, 211, 153, 0.1)' : 'rgba(16, 163, 74, 0.08)'
    case 'escalated': return isDark ? 'rgba(251, 191, 36, 0.1)' : 'rgba(230, 126, 34, 0.08)'
    default: return 'transparent'
  }
}

export function reviewDecisionIcon(d: string): string {
  switch (d) {
    case 'confirmed': return 'i-lucide-alert-triangle'
    case 'dismissed': return 'i-lucide-check-circle'
    case 'escalated': return 'i-lucide-arrow-up-circle'
    default: return 'i-lucide-circle'
  }
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
  colors: { errorBg: ColorFn, warningBg: ColorFn, infoBg: ColorFn }
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
// Monitoring Status Helpers (flagged / active / clean)
// ---------------------------------------------------------------------------

export type MonitoringStatus = 'flagged' | 'active' | 'clean'

export function monitoringStatusLabel(status: string): string {
  switch (status) {
    case 'flagged': return 'Подозрение'
    case 'active': return 'Активен'
    case 'clean': return 'Чисто'
    default: return status
  }
}

export function monitoringStatusColor(status: string): string {
  switch (status) {
    case 'flagged': return 'var(--argus-error)'
    case 'active': return 'var(--argus-warning)'
    case 'clean': return 'var(--argus-success)'
    default: return 'var(--argus-text-dimmed)'
  }
}

export function monitoringStatusBg(
  status: string,
  opacity: number,
  colors: { errorBg: ColorFn, warningBg: ColorFn, successBg: ColorFn }
): string {
  switch (status) {
    case 'flagged': return colors.errorBg(opacity)
    case 'active': return colors.warningBg(opacity)
    case 'clean': return colors.successBg(opacity)
    default: return 'var(--argus-bg-hover)'
  }
}

// ---------------------------------------------------------------------------
// Exam Status Helpers (active / completed / scheduled)
// ---------------------------------------------------------------------------

export type ExamStatus = 'active' | 'completed' | 'scheduled'

export function examStatusLabel(status: string): string {
  switch (status) {
    case 'active': return 'Активный'
    case 'completed': return 'Завершён'
    case 'scheduled': return 'Запланирован'
    default: return '—'
  }
}

export function examStatusColor(status: string): string {
  switch (status) {
    case 'active': return 'var(--argus-success)'
    case 'completed': return 'var(--argus-text-dimmed)'
    case 'scheduled': return 'var(--argus-accent)'
    default: return 'var(--argus-text-dimmed)'
  }
}

export function examStatusBg(
  status: string,
  opacity: number,
  colors: { successBg: ColorFn, accentBg: ColorFn },
  isDark: boolean
): string {
  switch (status) {
    case 'active': return colors.successBg(opacity)
    case 'completed': return isDark ? `rgba(148, 163, 184, ${opacity})` : `rgba(100, 116, 139, ${opacity})`
    case 'scheduled': return colors.accentBg(opacity)
    default: return 'transparent'
  }
}

// ---------------------------------------------------------------------------
// Test Status Helpers (active / draft / archived)
// ---------------------------------------------------------------------------

export type TestStatus = 'active' | 'draft' | 'archived'

export function testStatusLabel(status: string): string {
  switch (status) {
    case 'active': return 'Активный'
    case 'draft': return 'Черновик'
    case 'archived': return 'Архив'
    default: return '—'
  }
}

export function testStatusColor(status: string): string {
  switch (status) {
    case 'active': return 'var(--argus-success)'
    case 'draft': return 'var(--argus-warning)'
    case 'archived': return 'var(--argus-text-dimmed)'
    default: return 'var(--argus-text-dimmed)'
  }
}

export function testStatusBg(
  status: string,
  opacity: number,
  colors: { successBg: ColorFn, warningBg: ColorFn },
  isDark: boolean
): string {
  switch (status) {
    case 'active': return colors.successBg(opacity)
    case 'draft': return colors.warningBg(opacity)
    case 'archived': return isDark ? `rgba(148, 163, 184, ${opacity})` : `rgba(100, 116, 139, ${opacity})`
    default: return 'transparent'
  }
}

// ---------------------------------------------------------------------------
// Infrastructure Status Helpers (healthy / warning / critical)
// ---------------------------------------------------------------------------

export type InfraStatus = 'healthy' | 'warning' | 'critical'

export function infraStatusLabel(status: string): string {
  switch (status) {
    case 'healthy': return 'В норме'
    case 'warning': return 'Внимание'
    case 'critical': return 'Критично'
    default: return 'Неизвестно'
  }
}

export function infraStatusColor(status: string): string {
  switch (status) {
    case 'healthy': return 'var(--argus-success)'
    case 'warning': return 'var(--argus-warning)'
    case 'critical': return 'var(--argus-error)'
    default: return 'var(--argus-text-dimmed)'
  }
}

export function infraStatusBg(
  status: string,
  opacity: number,
  colors: { successBg: ColorFn, warningBg: ColorFn, errorBg: ColorFn }
): string {
  switch (status) {
    case 'healthy': return colors.successBg(opacity)
    case 'warning': return colors.warningBg(opacity)
    case 'critical': return colors.errorBg(opacity)
    default: return 'var(--argus-bg-hover)'
  }
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

export function exportStatusBg(
  status: string,
  opacity: number,
  colors: { warningBg: ColorFn, accentBg: ColorFn, successBg: ColorFn, errorBg: ColorFn },
  isDark: boolean
): string {
  switch (status) {
    case 'pending': return colors.warningBg(opacity)
    case 'processing': return colors.accentBg(opacity)
    case 'completed': return colors.successBg(opacity)
    case 'failed': return colors.errorBg(opacity)
    case 'expired': return isDark ? `rgba(100, 116, 139, ${opacity})` : `rgba(148, 163, 184, ${opacity})`
    default: return 'transparent'
  }
}

// ---------------------------------------------------------------------------
// Audio / Noise Level
// ---------------------------------------------------------------------------

/**
 * Returns CSS color variable for a noise level (dB).
 * Thresholds: >55 dB = error (red), >35 dB = warning (yellow), else = success (green).
 */
export function noiseLevelColor(level: number): string {
  if (level > 55) return 'var(--argus-error)'
  if (level > 35) return 'var(--argus-warning)'
  return 'var(--argus-success)'
}

/**
 * Returns human-readable label for a noise level (dB).
 */
export function noiseLevelLabel(level: number): string {
  if (level > 55) return 'Высокий'
  if (level > 35) return 'Средний'
  return 'Тихо'
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

    // Review decision
    reviewDecisionLabel,
    reviewDecisionColor,
    reviewDecisionBg: (d: string) => reviewDecisionBg(d, colors.isDark.value),
    reviewDecisionIcon,

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
    exportStatusIcon,
    exportStatusBg: (status: string, opacity: number) => exportStatusBg(status, opacity, colors, colors.isDark.value),

    // Audio / Noise level
    noiseLevelColor,
    noiseLevelLabel,

    // Monitoring status (flagged / active / clean)
    monitoringStatusLabel,
    monitoringStatusColor,
    monitoringStatusBg: (status: string, opacity: number) => monitoringStatusBg(status, opacity, colors),

    // Exam status (active / completed / scheduled)
    examStatusLabel,
    examStatusColor,
    examStatusBg: (status: string, opacity: number) => examStatusBg(status, opacity, colors, colors.isDark.value),

    // Test status (active / draft / archived)
    testStatusLabel,
    testStatusColor,
    testStatusBg: (status: string, opacity: number) => testStatusBg(status, opacity, colors, colors.isDark.value),

    // Infrastructure status (healthy / warning / critical)
    infraStatusLabel,
    infraStatusColor,
    infraStatusBg: (status: string, opacity: number) => infraStatusBg(status, opacity, colors)
  }
}
