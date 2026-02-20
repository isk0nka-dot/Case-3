<script setup lang="ts">
/**
 * IntegrityReport — Forensic Ledger Verification Panel
 *
 * Displays the SHA-256 hash chain integrity status for a proctoring session.
 * Features:
 * - One-click "Verify Integrity" button that calls POST /api/v1/integrity/verify-session
 * - Visual chain diagram showing each fragment's hash linkage
 * - Green/red indicators for chain validity and S3 hash match
 * - Broken link and mismatch details
 * - Duration and fragment count summary
 * - Cached results via GET /api/v1/integrity/session/{id}/report
 */
import {
  useAdminAPI,
  type IntegritySessionReport,
  type IntegrityFragmentResult
} from '~/composables/useAdminAPI'
import { useColors } from '~/composables/useColors'

const props = defineProps<{
  sessionId: string
}>()

const api = useAdminAPI()
const { isDark, accentBg, successBg, errorBg, warningBg } = useColors()

// State
const report = ref<IntegritySessionReport | null>(null)
const loading = ref(false)
const verifying = ref(false)
const error = ref('')
const expanded = ref(false)

// Try to load cached report
async function loadCachedReport() {
  loading.value = true
  error.value = ''
  try {
    report.value = await api.getIntegrityReport(props.sessionId)
  } catch {
    // No cached report — that's fine, user can trigger verification
    report.value = null
  } finally {
    loading.value = false
  }
}

// Verify session integrity (triggers full re-verification)
async function verifyIntegrity() {
  verifying.value = true
  error.value = ''
  try {
    report.value = await api.verifySessionIntegrity(props.sessionId)
  } catch (err: unknown) {
    error.value = err instanceof Error ? err.message : 'Verification failed'
  } finally {
    verifying.value = false
  }
}

// Verify a single fragment
const fragmentVerifying = ref<string | null>(null)
const fragmentResults = ref<Record<string, IntegrityFragmentResult>>({})

async function verifySingleFragment(fragmentId: string) {
  fragmentVerifying.value = fragmentId
  try {
    const result = await api.verifyFragmentIntegrity(fragmentId)
    fragmentResults.value[fragmentId] = result
  } catch {
    fragmentResults.value[fragmentId] = {
      fragmentId,
      sequenceNum: 0,
      sha256Match: false,
      chainValid: false,
      previousHash: '',
      recordHash: '',
      errorMessage: 'Verification failed'
    }
  } finally {
    fragmentVerifying.value = null
  }
}

function truncateHash(hash: string): string {
  if (!hash || hash.length < 16) return hash || '—'
  return hash.slice(0, 8) + '...' + hash.slice(-8)
}

// Overall integrity percentage
const integrityPercent = computed(() => {
  if (!report.value || report.value.totalFragments === 0) return 0
  return Math.round((report.value.verifiedOk / report.value.totalFragments) * 100)
})

// Overall status
const overallStatus = computed(() => {
  if (!report.value) return 'unknown'
  if (report.value.chainValid && report.value.s3Mismatches === 0 && report.value.brokenLinks.length === 0) return 'valid'
  if (report.value.brokenLinks.length > 0 || report.value.s3Mismatches > 0) return 'tampered'
  return 'partial'
})

onMounted(() => loadCachedReport())
watch(() => props.sessionId, () => {
  report.value = null
  error.value = ''
  expanded.value = false
  loadCachedReport()
})
</script>

<template>
  <div class="space-y-3">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2">
        <UIcon name="i-lucide-shield-check" class="size-4" style="color: var(--argus-accent);" />
        <span class="text-xs font-semibold" style="color: var(--argus-text);">Forensic Ledger</span>
        <span
          v-if="report"
          class="text-[9px] font-bold px-1.5 py-0.5 rounded-full"
          :style="{
            background: overallStatus === 'valid' ? successBg(0.1) : overallStatus === 'tampered' ? errorBg(0.1) : warningBg(0.1),
            color: overallStatus === 'valid' ? 'var(--argus-success)' : overallStatus === 'tampered' ? 'var(--argus-error)' : 'var(--argus-warning)'
          }"
        >
          {{ overallStatus === 'valid' ? 'VERIFIED' : overallStatus === 'tampered' ? 'TAMPERED' : 'PARTIAL' }}
        </span>
      </div>

      <button
        class="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-[10px] font-semibold transition-all border"
        :style="{
          background: verifying ? accentBg(0.15) : accentBg(0.08),
          borderColor: accentBg(0.2),
          color: 'var(--argus-accent)',
          opacity: verifying ? 0.7 : 1
        }"
        :disabled="verifying"
        @click="verifyIntegrity"
      >
        <div
          v-if="verifying"
          class="animate-spin rounded-full size-3 border border-t-transparent"
          style="border-color: var(--argus-accent); border-top-color: transparent;"
        />
        <UIcon v-else name="i-lucide-scan" class="size-3" />
        {{ verifying ? 'Проверка...' : 'Проверить целостность' }}
      </button>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="flex items-center justify-center py-4">
      <div class="animate-spin rounded-full size-5 border-2 border-t-transparent" style="border-color: var(--argus-accent); border-top-color: transparent;" />
    </div>

    <!-- Error -->
    <div
      v-else-if="error"
      class="flex items-center gap-2 px-3 py-2 rounded-lg text-[10px]"
      :style="{ background: errorBg(0.08), color: 'var(--argus-error)' }"
    >
      <UIcon name="i-lucide-alert-circle" class="size-3 shrink-0" />
      {{ error }}
    </div>

    <!-- No report yet -->
    <div
      v-else-if="!report"
      class="flex flex-col items-center justify-center py-6"
    >
      <UIcon name="i-lucide-shield-question" class="size-8 mb-2" style="color: var(--argus-text-dimmed);" />
      <p class="text-[10px] font-medium" style="color: var(--argus-text);">Верификация не проводилась</p>
      <p class="text-[9px] mt-0.5" style="color: var(--argus-text-dimmed);">Нажмите "Проверить целостность" для анализа хеш-цепочки</p>
    </div>

    <!-- Report Display -->
    <div v-else class="space-y-3">
      <!-- Summary Cards -->
      <div class="grid grid-cols-2 gap-2">
        <!-- Chain Status -->
        <div
          class="rounded-lg border p-3"
          :style="{
            borderColor: report.chainValid ? 'rgba(52, 211, 153, 0.2)' : 'rgba(248, 113, 113, 0.2)',
            background: report.chainValid ? successBg(0.04) : errorBg(0.04)
          }"
        >
          <div class="flex items-center gap-1.5 mb-1.5">
            <UIcon
              :name="report.chainValid ? 'i-lucide-link' : 'i-lucide-unlink'"
              class="size-3.5"
              :style="{ color: report.chainValid ? 'var(--argus-success)' : 'var(--argus-error)' }"
            />
            <span class="text-[9px] font-semibold uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Hash Chain</span>
          </div>
          <p class="text-sm font-bold" :style="{ color: report.chainValid ? 'var(--argus-success)' : 'var(--argus-error)' }">
            {{ report.chainValid ? 'VALID' : 'BROKEN' }}
          </p>
          <p class="text-[8px] mt-0.5" style="color: var(--argus-text-dimmed);">
            SHA-256 linked chain
          </p>
        </div>

        <!-- Fragment Count -->
        <div
          class="rounded-lg border p-3"
          :style="{ borderColor: 'var(--argus-border)', background: 'var(--argus-bg-elevated)' }"
        >
          <div class="flex items-center gap-1.5 mb-1.5">
            <UIcon name="i-lucide-layers" class="size-3.5" style="color: var(--argus-accent);" />
            <span class="text-[9px] font-semibold uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Fragments</span>
          </div>
          <p class="text-sm font-bold" style="color: var(--argus-text);">
            {{ report.verifiedOk }} / {{ report.totalFragments }}
          </p>
          <p class="text-[8px] mt-0.5" style="color: var(--argus-text-dimmed);">
            {{ integrityPercent }}% verified OK
          </p>
        </div>

        <!-- S3 Verification -->
        <div
          class="rounded-lg border p-3"
          :style="{
            borderColor: report.s3Mismatches === 0 ? 'rgba(52, 211, 153, 0.2)' : 'rgba(248, 113, 113, 0.2)',
            background: report.s3Mismatches === 0 ? successBg(0.04) : errorBg(0.04)
          }"
        >
          <div class="flex items-center gap-1.5 mb-1.5">
            <UIcon name="i-lucide-hard-drive" class="size-3.5" :style="{ color: report.s3Mismatches === 0 ? 'var(--argus-success)' : 'var(--argus-error)' }" />
            <span class="text-[9px] font-semibold uppercase tracking-wider" style="color: var(--argus-text-dimmed);">S3 / MinIO</span>
          </div>
          <p class="text-sm font-bold" :style="{ color: report.s3Mismatches === 0 ? 'var(--argus-success)' : 'var(--argus-error)' }">
            {{ report.s3Verified }} OK
          </p>
          <p class="text-[8px] mt-0.5" style="color: var(--argus-text-dimmed);">
            {{ report.s3Mismatches }} mismatches, {{ report.s3Errors }} errors
          </p>
        </div>

        <!-- Verification Time -->
        <div
          class="rounded-lg border p-3"
          :style="{ borderColor: 'var(--argus-border)', background: 'var(--argus-bg-elevated)' }"
        >
          <div class="flex items-center gap-1.5 mb-1.5">
            <UIcon name="i-lucide-timer" class="size-3.5" style="color: var(--argus-accent);" />
            <span class="text-[9px] font-semibold uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Duration</span>
          </div>
          <p class="text-sm font-bold tabular-nums" style="color: var(--argus-text);">
            {{ report.durationMs }}ms
          </p>
          <p class="text-[8px] mt-0.5" style="color: var(--argus-text-dimmed);">
            {{ new Date(report.verifiedAt).toLocaleTimeString('ru-RU') }}
          </p>
        </div>
      </div>

      <!-- Integrity Progress Bar -->
      <div class="rounded-lg border p-3" :style="{ borderColor: 'var(--argus-border)', background: 'var(--argus-bg-elevated)' }">
        <div class="flex items-center justify-between mb-2">
          <span class="text-[9px] font-semibold" style="color: var(--argus-text-dimmed);">Evidence Integrity</span>
          <span class="text-[10px] font-bold tabular-nums" :style="{ color: integrityPercent === 100 ? 'var(--argus-success)' : integrityPercent >= 80 ? 'var(--argus-warning)' : 'var(--argus-error)' }">
            {{ integrityPercent }}%
          </span>
        </div>
        <div class="w-full h-2 rounded-full overflow-hidden" style="background: var(--argus-bg-hover);">
          <div
            class="h-full rounded-full transition-all duration-500"
            :style="{
              width: `${integrityPercent}%`,
              background: integrityPercent === 100
                ? (isDark ? 'linear-gradient(90deg, #34D399, #10B981)' : 'linear-gradient(90deg, #10A34A, #0D8A3E)')
                : integrityPercent >= 80
                  ? (isDark ? 'linear-gradient(90deg, #FBBF24, #F59E0B)' : 'linear-gradient(90deg, #E67E22, #C96E1A)')
                  : (isDark ? 'linear-gradient(90deg, #F87171, #EF4444)' : 'linear-gradient(90deg, #E03E3E, #C92B2B)')
            }"
          />
        </div>
      </div>

      <!-- Broken Links / Mismatches (if any) -->
      <div
        v-if="report.brokenLinks.length > 0 || report.mismatches.length > 0"
        class="rounded-lg border p-3 space-y-2"
        :style="{ borderColor: 'rgba(248, 113, 113, 0.2)', background: errorBg(0.04) }"
      >
        <div class="flex items-center gap-1.5">
          <UIcon name="i-lucide-alert-triangle" class="size-3.5" style="color: var(--argus-error);" />
          <span class="text-[10px] font-bold" style="color: var(--argus-error);">
            Tamper Detection: {{ report.brokenLinks.length }} broken links, {{ report.mismatches.length }} hash mismatches
          </span>
        </div>

        <div
          v-for="item in [...report.brokenLinks, ...report.mismatches]"
          :key="item.fragmentId + '-issue'"
          class="flex items-center gap-2 px-2 py-1.5 rounded text-[9px]"
          :style="{ background: errorBg(0.06) }"
        >
          <UIcon
            :name="!item.chainValid ? 'i-lucide-unlink' : 'i-lucide-file-x'"
            class="size-3 shrink-0"
            style="color: var(--argus-error);"
          />
          <span class="font-mono" style="color: var(--argus-text-muted);">
            #{{ item.sequenceNum }}
          </span>
          <span style="color: var(--argus-text-dimmed);">{{ truncateHash(item.fragmentId) }}</span>
          <span
            v-if="!item.chainValid"
            class="font-bold px-1 py-0.5 rounded"
            :style="{ background: errorBg(0.1), color: 'var(--argus-error)' }"
          >
            CHAIN BREAK
          </span>
          <span
            v-if="!item.sha256Match"
            class="font-bold px-1 py-0.5 rounded"
            :style="{ background: errorBg(0.1), color: 'var(--argus-error)' }"
          >
            HASH MISMATCH
          </span>
          <span v-if="item.errorMessage" style="color: var(--argus-text-dimmed);">{{ item.errorMessage }}</span>
        </div>
      </div>

      <!-- Expandable Fragment Chain -->
      <div>
        <button
          class="flex items-center gap-1.5 text-[10px] font-medium transition-all"
          :style="{ color: 'var(--argus-accent)' }"
          @click="expanded = !expanded"
        >
          <UIcon
            name="i-lucide-chevron-down"
            class="size-3 transition-transform duration-200"
            :style="{ transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)' }"
          />
          {{ expanded ? 'Скрыть цепочку' : `Показать цепочку (${report.fragments.length} фрагментов)` }}
        </button>

        <div v-if="expanded" class="mt-2 space-y-1 max-h-[200px] overflow-y-auto">
          <div
            v-for="(frag, idx) in report.fragments"
            :key="frag.fragmentId"
            class="flex items-center gap-2 px-2 py-1.5 rounded text-[9px]"
            :style="{
              background: frag.sha256Match && frag.chainValid ? successBg(0.04) : errorBg(0.04),
              border: `1px solid ${frag.sha256Match && frag.chainValid ? 'rgba(52, 211, 153, 0.15)' : 'rgba(248, 113, 113, 0.15)'}`
            }"
          >
            <!-- Sequence number -->
            <span class="font-mono font-bold w-6 text-right shrink-0" style="color: var(--argus-text-dimmed);">
              {{ idx + 1 }}
            </span>

            <!-- Chain link icon -->
            <UIcon
              :name="frag.chainValid ? 'i-lucide-link' : 'i-lucide-unlink'"
              class="size-3 shrink-0"
              :style="{ color: frag.chainValid ? 'var(--argus-success)' : 'var(--argus-error)' }"
            />

            <!-- Hash -->
            <span class="font-mono flex-1 truncate" style="color: var(--argus-text-muted);">
              {{ truncateHash(frag.recordHash) }}
            </span>

            <!-- Status badges -->
            <span
              class="font-bold px-1 py-0.5 rounded shrink-0"
              :style="{
                background: frag.sha256Match ? successBg(0.1) : errorBg(0.1),
                color: frag.sha256Match ? 'var(--argus-success)' : 'var(--argus-error)'
              }"
            >
              {{ frag.sha256Match ? 'SHA OK' : 'SHA FAIL' }}
            </span>

            <!-- Per-fragment verify button -->
            <button
              class="flex items-center gap-0.5 px-1.5 py-0.5 rounded transition-all shrink-0"
              :style="{ background: 'var(--argus-bg-hover)', color: 'var(--argus-text-muted)' }"
              :disabled="fragmentVerifying === frag.fragmentId"
              @click.stop="verifySingleFragment(frag.fragmentId)"
            >
              <div
                v-if="fragmentVerifying === frag.fragmentId"
                class="animate-spin rounded-full size-2 border border-t-transparent"
                style="border-color: var(--argus-text-dimmed); border-top-color: transparent;"
              />
              <UIcon v-else name="i-lucide-scan" class="size-2.5" />
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
