<script setup lang="ts">
// =============================================================================
// Argus AI — Forensic Report Page
// =============================================================================
// Integrity audit portal for generating, viewing, and verifying forensic reports.
// Features:
//   - Session search by ID
//   - Full forensic report with integrity score, verdict, penalties
//   - Violation timeline
//   - Voice biometric analysis
//   - Gaze heatmap (SVG embed)
//   - Forensic ledger verification
//   - PDF download
//   - Report hash verification tool
// =============================================================================

import type {
  ForensicReport,
  ForensicIntegrityScore,
  ForensicVerifyResult
} from '~/composables/useAdminAPI'
import { integrityColor } from '~/composables/useStatusHelpers'

const api = useAdminAPI()
const { isDark, accentBg, errorBg, successBg, warningBg, infoBg } = useColors()

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

const sessionIdInput = ref('')
const loading = ref(false)
const report = ref<ForensicReport | null>(null)
const error = ref('')
const activeTab = ref<'overview' | 'timeline' | 'voice' | 'heatmap' | 'ledger' | 'verify'>('overview')

// Verify tab state
const verifySessionId = ref('')
const verifyHash = ref('')
const verifyLoading = ref(false)
const verifyResult = ref<ForensicVerifyResult | null>(null)
const verifyError = ref('')

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

async function generateReport() {
  const sid = sessionIdInput.value.trim()
  if (!sid) return

  loading.value = true
  error.value = ''
  report.value = null

  try {
    report.value = await api.getForensicReport(sid)
    activeTab.value = 'overview'
  } catch (e: any) {
    error.value = e?.message || 'Failed to generate report'
  } finally {
    loading.value = false
  }
}

function downloadPDF() {
  if (!report.value) return
  const url = api.getForensicPDFUrl(report.value.sessionId)
  window.open(url, '_blank')
}

async function verifyReport() {
  const sid = verifySessionId.value.trim()
  const hash = verifyHash.value.trim()
  if (!sid || !hash) return

  verifyLoading.value = true
  verifyError.value = ''
  verifyResult.value = null

  try {
    verifyResult.value = await api.verifyForensicReport(sid, hash)
  } catch (e: any) {
    verifyError.value = e?.message || 'Verification failed'
  } finally {
    verifyLoading.value = false
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function verdictColor(verdict: string): string {
  switch (verdict) {
    case 'clean': return 'var(--argus-success)'
    case 'warning': return 'var(--argus-warning)'
    case 'fraud': return 'var(--argus-error)'
    default: return 'var(--argus-text-dimmed)'
  }
}

function verdictBg(verdict: string): string {
  switch (verdict) {
    case 'clean': return successBg(0.12)
    case 'warning': return warningBg(0.12)
    case 'fraud': return errorBg(0.12)
    default: return accentBg(0.08)
  }
}

function verdictIcon(verdict: string): string {
  switch (verdict) {
    case 'clean': return 'i-lucide-shield-check'
    case 'warning': return 'i-lucide-alert-triangle'
    case 'fraud': return 'i-lucide-shield-x'
    default: return 'i-lucide-help-circle'
  }
}

function voiceVerdictColor(verdict: string): string {
  switch (verdict) {
    case 'consistent': return 'var(--argus-success)'
    case 'suspicious': return 'var(--argus-warning)'
    case 'anomalous': return 'var(--argus-error)'
    default: return 'var(--argus-text-dimmed)'
  }
}

function formatDuration(sec: number): string {
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (h > 0) return `${h}ч ${m}м`
  return `${m}м`
}

const heatmapUrl = computed(() => {
  if (!report.value) return ''
  return api.getForensicHeatmapUrl(report.value.sessionId)
})

const tabs = [
  { key: 'overview', label: 'Обзор', icon: 'i-lucide-file-text' },
  { key: 'timeline', label: 'Таймлайн', icon: 'i-lucide-clock' },
  { key: 'voice', label: 'Голос', icon: 'i-lucide-mic' },
  { key: 'heatmap', label: 'Тепловая карта', icon: 'i-lucide-eye' },
  { key: 'ledger', label: 'Леджер', icon: 'i-lucide-link' },
  { key: 'verify', label: 'Проверить', icon: 'i-lucide-check-circle' }
] as const
</script>

<template>
  <div class="min-h-screen px-4 py-6 max-w-7xl mx-auto">
    <!-- Header -->
    <div class="mb-6">
      <div class="flex items-center gap-3 mb-2">
        <div
          class="flex items-center justify-center size-10 rounded-xl"
          :style="{ background: accentBg(0.12) }"
        >
          <UIcon
            name="i-lucide-file-search"
            class="size-5"
            style="color: var(--argus-accent);"
          />
        </div>
        <div>
          <h1
            class="text-xl font-bold"
            style="color: var(--argus-text);"
          >
            Форензик-отчёт
          </h1>
          <p
            class="text-xs"
            style="color: var(--argus-text-dimmed);"
          >
            Генерация и верификация отчётов целостности
          </p>
        </div>
      </div>
    </div>

    <!-- Search Bar -->
    <div
      class="rounded-xl border p-4 mb-6"
      :style="{
        background: isDark ? 'rgba(11, 15, 20, 0.6)' : 'rgba(255, 255, 255, 0.8)',
        borderColor: 'var(--argus-border)'
      }"
    >
      <div class="flex items-center gap-3">
        <div class="flex-1">
          <UInput
            v-model="sessionIdInput"
            placeholder="Введите Session ID..."
            size="lg"
            icon="i-lucide-search"
            @keydown.enter="generateReport"
          />
        </div>
        <UButton
          :loading="loading"
          icon="i-lucide-file-search"
          size="lg"
          @click="generateReport"
        >
          Сгенерировать
        </UButton>
      </div>

      <p
        v-if="error"
        class="mt-2 text-sm"
        style="color: var(--argus-error);"
      >
        {{ error }}
      </p>
    </div>

    <!-- Report Content -->
    <template v-if="report">
      <!-- Verdict Banner -->
      <div
        class="rounded-xl border p-5 mb-6"
        :style="{
          background: verdictBg(report.integrity.verdict),
          borderColor: verdictColor(report.integrity.verdict)
        }"
      >
        <div class="flex items-center gap-4 flex-wrap">
          <!-- Score -->
          <div class="flex items-center gap-3">
            <div
              class="flex items-center justify-center size-14 rounded-2xl"
              :style="{ background: verdictBg(report.integrity.verdict) }"
            >
              <UIcon
                :name="verdictIcon(report.integrity.verdict)"
                class="size-8"
                :style="{ color: verdictColor(report.integrity.verdict) }"
              />
            </div>
            <div>
              <p
                class="text-3xl font-black"
                :style="{ color: verdictColor(report.integrity.verdict) }"
              >
                {{ report.integrity.score.toFixed(1) }}%
              </p>
              <p
                class="text-xs font-semibold uppercase tracking-wider"
                :style="{ color: verdictColor(report.integrity.verdict) }"
              >
                {{ report.integrity.verdictLabel }}
              </p>
            </div>
          </div>

          <!-- Justification -->
          <div
            class="flex-1 min-w-0 pl-4 border-l"
            :style="{ borderColor: verdictColor(report.integrity.verdict) }"
          >
            <p
              class="text-sm"
              style="color: var(--argus-text);"
            >
              {{ report.integrity.justification }}
            </p>
          </div>

          <!-- PDF Download -->
          <UButton
            icon="i-lucide-download"
            variant="outline"
            size="lg"
            @click="downloadPDF"
          >
            PDF
          </UButton>
        </div>
      </div>

      <!-- Stats Row -->
      <div class="grid grid-cols-2 md:grid-cols-5 gap-3 mb-6">
        <div
          v-for="stat in [
            { label: 'Всего событий', value: report.integrity.totalEvents, bg: accentBg(0.08) },
            { label: 'Критических', value: report.integrity.criticalCount, bg: errorBg(0.08) },
            { label: 'Предупреждений', value: report.integrity.warningCount, bg: warningBg(0.08) },
            { label: 'Длительность', value: formatDuration(report.integrity.durationSec), bg: infoBg(0.08) },
            { label: 'Штрафов', value: report.integrity.penalties.length, bg: errorBg(0.08) }
          ]"
          :key="stat.label"
          class="rounded-xl border p-3 text-center"
          :style="{ background: stat.bg, borderColor: 'var(--argus-border)' }"
        >
          <p
            class="text-lg font-bold"
            style="color: var(--argus-text);"
          >
            {{ stat.value }}
          </p>
          <p
            class="text-[10px]"
            style="color: var(--argus-text-dimmed);"
          >
            {{ stat.label }}
          </p>
        </div>
      </div>

      <!-- Tab Navigation -->
      <div class="flex items-center gap-1 mb-4 overflow-x-auto pb-1">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-semibold transition-all whitespace-nowrap"
          :style="{
            background: activeTab === tab.key ? accentBg(0.15) : 'transparent',
            color: activeTab === tab.key ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)'
          }"
          @click="activeTab = tab.key"
        >
          <UIcon
            :name="tab.icon"
            class="size-3.5"
          />
          {{ tab.label }}
        </button>
      </div>

      <!-- Tab: Overview -->
      <div
        v-if="activeTab === 'overview'"
        class="space-y-4"
      >
        <!-- Session Metadata -->
        <div
          class="rounded-xl border p-4"
          :style="{
            background: isDark ? 'rgba(11, 15, 20, 0.6)' : 'rgba(255, 255, 255, 0.8)',
            borderColor: 'var(--argus-border)'
          }"
        >
          <h3
            class="text-sm font-bold mb-3"
            style="color: var(--argus-text);"
          >
            <UIcon
              name="i-lucide-info"
              class="size-4 inline mr-1"
            />
            Метаданные сессии
          </h3>
          <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
            <div
              v-for="item in [
                { label: 'Session ID', value: report.sessionId },
                { label: 'Student ID', value: report.studentId },
                { label: 'Exam ID', value: report.examId },
                { label: 'Организация', value: report.orgId },
                { label: 'User Agent', value: report.deviceInfo.userAgent?.substring(0, 40) + '...' },
                { label: 'Разрешение', value: report.deviceInfo.resolution },
                { label: 'IP-адрес', value: report.deviceInfo.ipAddress },
                { label: 'Регион', value: report.deviceInfo.region }
              ]"
              :key="item.label"
            >
              <p
                class="text-[10px]"
                style="color: var(--argus-text-dimmed);"
              >
                {{ item.label }}
              </p>
              <p
                class="text-xs font-mono truncate"
                style="color: var(--argus-text);"
              >
                {{ item.value || '—' }}
              </p>
            </div>
          </div>
        </div>

        <!-- Penalty Breakdown -->
        <div
          class="rounded-xl border p-4"
          :style="{
            background: isDark ? 'rgba(11, 15, 20, 0.6)' : 'rgba(255, 255, 255, 0.8)',
            borderColor: 'var(--argus-border)'
          }"
        >
          <h3
            class="text-sm font-bold mb-3"
            style="color: var(--argus-text);"
          >
            <UIcon
              name="i-lucide-minus-circle"
              class="size-4 inline mr-1"
            />
            Штрафы ({{ report.integrity.penalties.length }})
          </h3>

          <div
            v-if="report.integrity.penalties.length === 0"
            class="text-center py-6"
          >
            <UIcon
              name="i-lucide-check-circle"
              class="size-8 mx-auto mb-2"
              style="color: var(--argus-success);"
            />
            <p
              class="text-sm"
              style="color: var(--argus-text-dimmed);"
            >
              Нарушений не обнаружено
            </p>
          </div>

          <div
            v-else
            class="space-y-2"
          >
            <div
              v-for="(penalty, idx) in report.integrity.penalties"
              :key="idx"
              class="flex items-center gap-3 px-3 py-2 rounded-lg"
              :style="{ background: errorBg(0.06) }"
            >
              <span
                class="text-xs font-mono w-8 text-right font-bold"
                style="color: var(--argus-error);"
              >
                -{{ penalty.applied.toFixed(1) }}
              </span>
              <span
                class="flex-1 text-xs"
                style="color: var(--argus-text);"
              >
                {{ penalty.description }}
              </span>
              <span
                class="text-[10px] font-mono"
                style="color: var(--argus-text-dimmed);"
              >
                {{ penalty.count }}x @ -{{ penalty.penaltyPer }}
              </span>
              <span
                v-if="penalty.maxPenalty > 0"
                class="text-[10px]"
                style="color: var(--argus-text-dimmed);"
              >
                (max -{{ penalty.maxPenalty }})
              </span>
            </div>
          </div>
        </div>

        <!-- Top Factors -->
        <div
          v-if="report.integrity.topFactors.length > 0"
          class="rounded-xl border p-4"
          :style="{
            background: isDark ? 'rgba(11, 15, 20, 0.6)' : 'rgba(255, 255, 255, 0.8)',
            borderColor: 'var(--argus-border)'
          }"
        >
          <h3
            class="text-sm font-bold mb-3"
            style="color: var(--argus-text);"
          >
            <UIcon
              name="i-lucide-trending-up"
              class="size-4 inline mr-1"
            />
            Ключевые факторы
          </h3>
          <ol class="list-decimal list-inside space-y-1">
            <li
              v-for="(factor, idx) in report.integrity.topFactors"
              :key="idx"
              class="text-xs"
              style="color: var(--argus-text);"
            >
              {{ factor }}
            </li>
          </ol>
        </div>

        <!-- Report Hash -->
        <div
          class="rounded-xl border p-4"
          :style="{
            background: isDark ? 'rgba(11, 15, 20, 0.6)' : 'rgba(255, 255, 255, 0.8)',
            borderColor: 'var(--argus-border)'
          }"
        >
          <h3
            class="text-sm font-bold mb-2"
            style="color: var(--argus-text);"
          >
            <UIcon
              name="i-lucide-lock"
              class="size-4 inline mr-1"
            />
            Цифровая подпись
          </h3>
          <div class="flex items-center gap-2">
            <code
              class="text-[10px] font-mono break-all flex-1"
              style="color: var(--argus-text-dimmed);"
            >
              SHA-256: {{ report.reportHash }}
            </code>
          </div>
          <p
            class="text-[10px] mt-1"
            style="color: var(--argus-text-dimmed);"
          >
            Report ID: {{ report.reportId }} | {{ report.generatedAt }}
          </p>
        </div>
      </div>

      <!-- Tab: Timeline -->
      <div v-else-if="activeTab === 'timeline'">
        <div
          class="rounded-xl border p-4"
          :style="{
            background: isDark ? 'rgba(11, 15, 20, 0.6)' : 'rgba(255, 255, 255, 0.8)',
            borderColor: 'var(--argus-border)'
          }"
        >
          <h3
            class="text-sm font-bold mb-3"
            style="color: var(--argus-text);"
          >
            Нарушения ({{ report.timeline.length }})
          </h3>

          <div
            v-if="report.timeline.length === 0"
            class="text-center py-8"
          >
            <UIcon
              name="i-lucide-check"
              class="size-8 mx-auto mb-2"
              style="color: var(--argus-success);"
            />
            <p
              class="text-sm"
              style="color: var(--argus-text-dimmed);"
            >
              Нет нарушений
            </p>
          </div>

          <div
            v-else
            class="space-y-1 max-h-[600px] overflow-y-auto"
          >
            <div
              v-for="(entry, idx) in report.timeline"
              :key="idx"
              class="flex items-center gap-3 px-3 py-2 rounded-lg"
              :style="{ background: entry.severity === 'critical' ? errorBg(0.06) : warningBg(0.04) }"
            >
              <span
                class="size-2 rounded-full shrink-0"
                :style="{ background: entry.severity === 'critical' ? 'var(--argus-error)' : 'var(--argus-warning)' }"
              />
              <span
                class="text-[10px] font-mono w-14 shrink-0"
                style="color: var(--argus-text-dimmed);"
              >
                {{ entry.videoSec }}s
              </span>
              <span
                class="text-xs flex-1 truncate"
                style="color: var(--argus-text);"
              >
                {{ entry.label || entry.eventType }}
              </span>
              <span
                class="text-[10px] font-mono"
                style="color: var(--argus-text-dimmed);"
              >
                {{ (entry.confidence * 100).toFixed(0) }}%
              </span>
              <span
                class="text-[10px] w-12 text-right"
                style="color: var(--argus-text-dimmed);"
              >
                {{ entry.source }}
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- Tab: Voice -->
      <div v-else-if="activeTab === 'voice'">
        <div
          class="rounded-xl border p-4"
          :style="{
            background: isDark ? 'rgba(11, 15, 20, 0.6)' : 'rgba(255, 255, 255, 0.8)',
            borderColor: 'var(--argus-border)'
          }"
        >
          <div class="flex items-center gap-3 mb-4">
            <div
              class="flex items-center justify-center size-10 rounded-xl"
              :style="{ background: voiceVerdictColor(report.voiceBiometric.verdict) + '20' }"
            >
              <UIcon
                name="i-lucide-mic"
                class="size-5"
                :style="{ color: voiceVerdictColor(report.voiceBiometric.verdict) }"
              />
            </div>
            <div>
              <h3
                class="text-sm font-bold"
                style="color: var(--argus-text);"
              >
                Голосовая биометрия
              </h3>
              <p
                class="text-xs uppercase font-semibold"
                :style="{ color: voiceVerdictColor(report.voiceBiometric.verdict) }"
              >
                {{ report.voiceBiometric.verdict === 'consistent' ? 'Консистентный' : report.voiceBiometric.verdict === 'suspicious' ? 'Подозрительный' : 'Аномальный' }}
              </p>
            </div>
          </div>

          <div class="grid grid-cols-2 md:grid-cols-3 gap-4">
            <div
              v-for="item in [
                { label: 'Всего сегментов', value: report.voiceBiometric.totalSegments },
                { label: 'Совпадающих', value: report.voiceBiometric.matchedSegments },
                { label: 'Несовпадающих', value: report.voiceBiometric.mismatchedSegments },
                { label: 'Смен спикера', value: report.voiceBiometric.speakerChangeCount },
                { label: 'Консистентность', value: (report.voiceBiometric.consistencyScore * 100).toFixed(1) + '%' },
                { label: 'Осн. спикер', value: (report.voiceBiometric.primarySpeakerRatio * 100).toFixed(1) + '%' }
              ]"
              :key="item.label"
              class="rounded-lg p-3"
              :style="{ background: accentBg(0.06) }"
            >
              <p
                class="text-lg font-bold"
                style="color: var(--argus-text);"
              >
                {{ item.value }}
              </p>
              <p
                class="text-[10px]"
                style="color: var(--argus-text-dimmed);"
              >
                {{ item.label }}
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- Tab: Heatmap -->
      <div v-else-if="activeTab === 'heatmap'">
        <div
          class="rounded-xl border p-4"
          :style="{
            background: isDark ? 'rgba(11, 15, 20, 0.6)' : 'rgba(255, 255, 255, 0.8)',
            borderColor: 'var(--argus-border)'
          }"
        >
          <h3
            class="text-sm font-bold mb-3"
            style="color: var(--argus-text);"
          >
            Тепловая карта взгляда ({{ report.gazeData.length }} точек)
          </h3>

          <div
            v-if="report.gazeData.length === 0"
            class="text-center py-8"
          >
            <UIcon
              name="i-lucide-eye-off"
              class="size-8 mx-auto mb-2"
              style="color: var(--argus-text-dimmed);"
            />
            <p
              class="text-sm"
              style="color: var(--argus-text-dimmed);"
            >
              Нет данных взгляда
            </p>
          </div>

          <div
            v-else
            class="flex justify-center"
          >
            <img
              :src="heatmapUrl"
              alt="Gaze Heatmap"
              class="rounded-lg border max-w-full"
              :style="{ borderColor: 'var(--argus-border)' }"
            >
          </div>
        </div>
      </div>

      <!-- Tab: Ledger -->
      <div v-else-if="activeTab === 'ledger'">
        <div
          class="rounded-xl border p-4"
          :style="{
            background: isDark ? 'rgba(11, 15, 20, 0.6)' : 'rgba(255, 255, 255, 0.8)',
            borderColor: 'var(--argus-border)'
          }"
        >
          <h3
            class="text-sm font-bold mb-4"
            style="color: var(--argus-text);"
          >
            Форензик-леджер
          </h3>

          <div class="grid grid-cols-2 md:grid-cols-3 gap-4 mb-4">
            <div
              class="rounded-lg p-3 text-center"
              :style="{ background: accentBg(0.08) }"
            >
              <p
                class="text-2xl font-bold"
                style="color: var(--argus-text);"
              >
                {{ report.ledgerSummary.totalFragments }}
              </p>
              <p
                class="text-[10px]"
                style="color: var(--argus-text-dimmed);"
              >
                Всего фрагментов
              </p>
            </div>
            <div
              class="rounded-lg p-3 text-center"
              :style="{ background: successBg(0.08) }"
            >
              <p
                class="text-2xl font-bold"
                style="color: var(--argus-success);"
              >
                {{ report.ledgerSummary.verifiedOk }}
              </p>
              <p
                class="text-[10px]"
                style="color: var(--argus-text-dimmed);"
              >
                Верифицировано
              </p>
            </div>
            <div
              class="rounded-lg p-3 text-center"
              :style="{ background: report.ledgerSummary.chainValid ? successBg(0.08) : errorBg(0.08) }"
            >
              <UIcon
                :name="report.ledgerSummary.chainValid ? 'i-lucide-check-circle' : 'i-lucide-x-circle'"
                class="size-6 mx-auto mb-1"
                :style="{ color: report.ledgerSummary.chainValid ? 'var(--argus-success)' : 'var(--argus-error)' }"
              />
              <p
                class="text-[10px]"
                style="color: var(--argus-text-dimmed);"
              >
                Цепочка {{ report.ledgerSummary.chainValid ? 'валидна' : 'нарушена' }}
              </p>
            </div>
          </div>

          <div class="grid grid-cols-2 gap-4">
            <div
              class="rounded-lg p-3"
              :style="{ background: successBg(0.06) }"
            >
              <p
                class="text-lg font-bold"
                style="color: var(--argus-success);"
              >
                {{ report.ledgerSummary.s3Verified }}
              </p>
              <p
                class="text-[10px]"
                style="color: var(--argus-text-dimmed);"
              >
                S3 верифицировано
              </p>
            </div>
            <div
              class="rounded-lg p-3"
              :style="{ background: report.ledgerSummary.s3Mismatches > 0 ? errorBg(0.06) : successBg(0.06) }"
            >
              <p
                class="text-lg font-bold"
                :style="{ color: report.ledgerSummary.s3Mismatches > 0 ? 'var(--argus-error)' : 'var(--argus-success)' }"
              >
                {{ report.ledgerSummary.s3Mismatches }}
              </p>
              <p
                class="text-[10px]"
                style="color: var(--argus-text-dimmed);"
              >
                S3 несоответствий
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- Tab: Verify -->
      <div v-else-if="activeTab === 'verify'">
        <div
          class="rounded-xl border p-4"
          :style="{
            background: isDark ? 'rgba(11, 15, 20, 0.6)' : 'rgba(255, 255, 255, 0.8)',
            borderColor: 'var(--argus-border)'
          }"
        >
          <h3
            class="text-sm font-bold mb-4"
            style="color: var(--argus-text);"
          >
            <UIcon
              name="i-lucide-check-circle"
              class="size-4 inline mr-1"
            />
            Верификация отчёта
          </h3>
          <p
            class="text-xs mb-4"
            style="color: var(--argus-text-dimmed);"
          >
            Проверьте подлинность отчёта, сравнив его SHA-256 хеш с данными леджера.
          </p>

          <div class="space-y-3">
            <UInput
              v-model="verifySessionId"
              placeholder="Session ID"
              icon="i-lucide-hash"
            />
            <UInput
              v-model="verifyHash"
              placeholder="SHA-256 Report Hash"
              icon="i-lucide-lock"
            />
            <UButton
              :loading="verifyLoading"
              icon="i-lucide-check"
              @click="verifyReport"
            >
              Проверить
            </UButton>
          </div>

          <p
            v-if="verifyError"
            class="mt-3 text-sm"
            style="color: var(--argus-error);"
          >
            {{ verifyError }}
          </p>

          <div
            v-if="verifyResult"
            class="mt-4 rounded-lg p-4"
            :style="{ background: verifyResult.verified ? successBg(0.1) : errorBg(0.1) }"
          >
            <div class="flex items-center gap-2 mb-2">
              <UIcon
                :name="verifyResult.verified ? 'i-lucide-check-circle' : 'i-lucide-x-circle'"
                class="size-5"
                :style="{ color: verifyResult.verified ? 'var(--argus-success)' : 'var(--argus-error)' }"
              />
              <span
                class="text-sm font-bold"
                :style="{ color: verifyResult.verified ? 'var(--argus-success)' : 'var(--argus-error)' }"
              >
                {{ verifyResult.verified ? 'Отчёт подлинный' : 'Отчёт изменён или недействителен' }}
              </span>
            </div>
            <p
              class="text-[10px] font-mono"
              style="color: var(--argus-text-dimmed);"
            >
              Ожидаемый: {{ verifyResult.expected }}
            </p>
            <p
              class="text-[10px] font-mono"
              style="color: var(--argus-text-dimmed);"
            >
              Полученный: {{ verifyResult.submitted }}
            </p>
          </div>
        </div>
      </div>
    </template>

    <!-- Empty State -->
    <div
      v-else-if="!loading"
      class="text-center py-16"
    >
      <UIcon
        name="i-lucide-file-search"
        class="size-12 mx-auto mb-3"
        style="color: var(--argus-text-dimmed);"
      />
      <p
        class="text-sm"
        style="color: var(--argus-text-dimmed);"
      >
        Введите Session ID для генерации форензик-отчёта
      </p>
    </div>

    <!-- Loading State -->
    <div
      v-if="loading"
      class="text-center py-16"
    >
      <UIcon
        name="i-lucide-loader-2"
        class="size-8 mx-auto mb-3 animate-spin"
        style="color: var(--argus-accent);"
      />
      <p
        class="text-sm"
        style="color: var(--argus-text-dimmed);"
      >
        Генерация отчёта...
      </p>
    </div>
  </div>
</template>
