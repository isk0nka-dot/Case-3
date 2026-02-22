<script setup lang="ts">
/**
 * EvidenceViewer — displays evidence fragments for a proctoring session.
 *
 * Features:
 * - Lists video evidence clips as timeline cards
 * - Click a fragment to fetch presigned URL and play in <video> element
 * - SHA-256 hash display per fragment
 * - "Verify Integrity" button that re-hashes and compares
 * - Shows linked violation event context
 */
import { useAdminAPI, type EvidenceFragment, type PresignedURLResponse } from '~/composables/useAdminAPI'

const props = defineProps<{
  sessionId: string
}>()

const api = useAdminAPI()
const colorMode = useColorMode()
const isDark = computed(() => colorMode.value === 'dark')

// State
const fragments = ref<EvidenceFragment[]>([])
const loading = ref(false)
const error = ref('')
const activeFragment = ref<EvidenceFragment | null>(null)
const videoUrl = ref('')
const videoLoading = ref(false)
const verifying = ref<string | null>(null)
const verifyResults = ref<Record<string, { valid: boolean; checkedAt: string }>>({})

// Fetch evidence fragments for session
async function fetchEvidence() {
  loading.value = true
  error.value = ''
  try {
    const resp = await api.getSessionEvidence(props.sessionId)
    fragments.value = resp.fragments || []
  } catch (err: any) {
    error.value = err.message || 'Failed to load evidence'
    fragments.value = []
  } finally {
    loading.value = false
  }
}

// Play a specific fragment
async function playFragment(fragment: EvidenceFragment) {
  activeFragment.value = fragment
  videoUrl.value = ''
  videoLoading.value = true
  try {
    const resp: PresignedURLResponse = await api.getEvidencePresignedURL(fragment.fragmentId)
    videoUrl.value = resp.url
  } catch (err: any) {
    error.value = `Failed to get video URL: ${err.message}`
  } finally {
    videoLoading.value = false
  }
}

// Verify integrity of a fragment
async function verifyFragment(fragmentId: string) {
  verifying.value = fragmentId
  try {
    const resp = await api.verifyEvidence(fragmentId)
    verifyResults.value[fragmentId] = {
      valid: resp.valid,
      checkedAt: resp.verifiedAt
    }
  } catch (err: any) {
    verifyResults.value[fragmentId] = {
      valid: false,
      checkedAt: new Date().toISOString()
    }
  } finally {
    verifying.value = null
  }
}

// formatFileSize → centralized in useFormatters() composable
const { formatFileSize } = useFormatters()

function formatDuration(sec: number): string {
  const m = Math.floor(sec / 60)
  const s = Math.floor(sec % 60)
  return `${m}:${String(s).padStart(2, '0')}`
}

function truncateHash(hash: string): string {
  if (!hash || hash.length < 16) return hash
  return hash.slice(0, 8) + '...' + hash.slice(-8)
}

// Fetch on mount
onMounted(() => fetchEvidence())

// Re-fetch when sessionId changes
watch(() => props.sessionId, () => fetchEvidence())
</script>

<template>
  <div class="space-y-3">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2">
        <UIcon name="i-lucide-film" class="size-4" style="color: var(--argus-accent);" />
        <span class="text-xs font-semibold" style="color: var(--argus-text);">Видеодоказательства</span>
        <span
          v-if="!loading"
          class="text-[9px] font-bold px-1.5 py-0.5 rounded-full"
          :style="{
            background: fragments.length > 0
              ? (isDark ? 'rgba(56, 189, 248, 0.1)' : 'rgba(37, 99, 235, 0.1)')
              : 'var(--argus-bg-hover)',
            color: fragments.length > 0 ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)'
          }"
        >
          {{ fragments.length }} фрагмент{{ fragments.length !== 1 ? 'ов' : '' }}
        </span>
      </div>
      <button
        v-if="!loading"
        class="text-[9px] font-medium px-2 py-1 rounded transition-all"
        :style="{ color: 'var(--argus-accent)' }"
        @click="fetchEvidence"
      >
        Обновить
      </button>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="flex items-center justify-center py-6">
      <div class="animate-spin rounded-full size-5 border-2 border-t-transparent" style="border-color: var(--argus-accent); border-top-color: transparent;" />
    </div>

    <!-- Error -->
    <div
      v-else-if="error && fragments.length === 0"
      class="text-center py-6"
    >
      <UIcon name="i-lucide-alert-circle" class="size-8 mb-2" style="color: var(--argus-text-dimmed);" />
      <p class="text-xs" style="color: var(--argus-text-dimmed);">{{ error }}</p>
    </div>

    <!-- Empty state -->
    <div
      v-else-if="fragments.length === 0"
      class="text-center py-6"
    >
      <UIcon name="i-lucide-video-off" class="size-8 mb-2" style="color: var(--argus-text-dimmed);" />
      <p class="text-xs font-medium" style="color: var(--argus-text);">Нет видеодоказательств</p>
      <p class="text-[10px] mt-1" style="color: var(--argus-text-dimmed);">Для данной сессии видеозаписи не были зафиксированы</p>
    </div>

    <!-- Fragment List -->
    <div v-else class="space-y-2">
      <!-- Active Video Player -->
      <div
        v-if="activeFragment"
        class="rounded-lg border overflow-hidden"
        :style="{ borderColor: 'var(--argus-border)', background: 'var(--argus-bg-deep)' }"
      >
        <div v-if="videoLoading" class="flex items-center justify-center py-12">
          <div class="animate-spin rounded-full size-6 border-2 border-t-transparent" style="border-color: var(--argus-accent); border-top-color: transparent;" />
        </div>
        <video
          v-else-if="videoUrl"
          :src="videoUrl"
          controls
          class="w-full max-h-[240px] bg-black"
          preload="metadata"
        />
        <div class="px-3 py-2 flex items-center justify-between" style="border-top: 1px solid var(--argus-border);">
          <div class="flex items-center gap-2">
            <span class="text-[9px] font-mono" style="color: var(--argus-text-dimmed);">
              SHA-256: {{ truncateHash(activeFragment.sha256Hash) }}
            </span>
          </div>
          <button
            class="text-[9px] font-medium px-2 py-1 rounded"
            style="color: var(--argus-text-dimmed);"
            @click="activeFragment = null; videoUrl = ''"
          >
            Закрыть
          </button>
        </div>
      </div>

      <!-- Fragment Cards -->
      <div
        v-for="frag in fragments"
        :key="frag.fragmentId"
        class="flex items-center gap-3 px-3 py-2.5 rounded-lg border transition-all cursor-pointer"
        :style="{
          borderColor: activeFragment?.fragmentId === frag.fragmentId ? 'var(--argus-accent)' : 'var(--argus-border)',
          background: activeFragment?.fragmentId === frag.fragmentId
            ? (isDark ? 'rgba(56, 189, 248, 0.05)' : 'rgba(37, 99, 235, 0.03)')
            : 'var(--argus-bg-elevated)'
        }"
        @click="playFragment(frag)"
      >
        <!-- Play icon -->
        <div
          class="flex items-center justify-center size-8 rounded-lg shrink-0"
          :style="{
            background: isDark ? 'rgba(56, 189, 248, 0.1)' : 'rgba(37, 99, 235, 0.08)',
            color: 'var(--argus-accent)'
          }"
        >
          <UIcon name="i-lucide-play" class="size-3.5" />
        </div>

        <!-- Fragment info -->
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2">
            <span class="text-[10px] font-semibold" style="color: var(--argus-text);">
              {{ formatDuration(frag.durationSec) }}
            </span>
            <span class="text-[9px]" style="color: var(--argus-text-dimmed);">
              {{ formatTime(frag.startTime) }} — {{ formatTime(frag.endTime) }}
            </span>
          </div>
          <div class="flex items-center gap-2 mt-0.5">
            <span class="text-[8px] font-mono" style="color: var(--argus-text-dimmed);">
              {{ truncateHash(frag.sha256Hash) }}
            </span>
            <span class="text-[8px]" style="color: var(--argus-border);">|</span>
            <span class="text-[8px]" style="color: var(--argus-text-dimmed);">
              {{ formatFileSize(frag.sizeBytes) }}
            </span>
          </div>
        </div>

        <!-- Verify button -->
        <div class="flex items-center gap-1.5 shrink-0">
          <!-- Verify result badge -->
          <span
            v-if="frag.fragmentId in verifyResults"
            class="text-[8px] font-bold px-1.5 py-0.5 rounded-full"
            :style="{
              background: verifyResults[frag.fragmentId]?.valid
                ? (isDark ? 'rgba(52, 211, 153, 0.1)' : 'rgba(16, 163, 74, 0.08)')
                : (isDark ? 'rgba(248, 113, 113, 0.1)' : 'rgba(224, 62, 62, 0.08)'),
              color: verifyResults[frag.fragmentId]?.valid ? 'var(--argus-success)' : 'var(--argus-error)'
            }"
          >
            {{ verifyResults[frag.fragmentId]?.valid ? 'VALID' : 'INVALID' }}
          </span>

          <button
            class="flex items-center gap-1 px-2 py-1 rounded text-[8px] font-medium transition-all"
            :style="{
              background: 'var(--argus-bg-hover)',
              color: 'var(--argus-text-muted)'
            }"
            :disabled="verifying === frag.fragmentId"
            @click.stop="verifyFragment(frag.fragmentId)"
          >
            <div
              v-if="verifying === frag.fragmentId"
              class="animate-spin rounded-full size-2.5 border border-t-transparent"
              style="border-color: var(--argus-text-dimmed); border-top-color: transparent;"
            />
            <UIcon v-else name="i-lucide-shield-check" class="size-2.5" />
            Проверить
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
