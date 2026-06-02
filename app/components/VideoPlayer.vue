<script setup lang="ts">
// =============================================================================
// Argus AI — VideoPlayer Component (LiveKit WebRTC)
// =============================================================================
// Connects to a LiveKit room and subscribes to the student's video track.
// Renders the live camera feed with connection state indicators.
// =============================================================================

import {
  Room,
  RoomEvent,
  Track,
  ConnectionState,
  type RemoteTrackPublication,
  type RemoteParticipant
} from 'livekit-client'
import { useAdminAPI } from '~/composables/useAdminAPI'

const props = defineProps<{
  sessionId: string
  compact?: boolean // true = small card, false = full-size modal
}>()

const emit = defineEmits<{
  (e: 'connected'): void
  (e: 'disconnected'): void
  (e: 'error', err: string): void
}>()

const api = useAdminAPI()

// --- State ---
const videoRef = ref<HTMLVideoElement | null>(null)
const connectionState = ref<'idle' | 'connecting' | 'connected' | 'reconnecting' | 'disconnected' | 'error'>('idle')
const participantCount = ref(0)
const hasVideo = ref(false)
const errorMessage = ref('')

let room: Room | null = null

// --- Connect to LiveKit ---
async function connect() {
  if (room) return // Already connected

  connectionState.value = 'connecting'
  errorMessage.value = ''

  try {
    // Get token from our backend
    const tokenResp = await api.getMediaToken(props.sessionId)
    if (!tokenResp || !tokenResp.token) {
      throw new Error('Failed to get media token')
    }

    // Create LiveKit room
    room = new Room({
      adaptiveStream: true,
      dynacast: true,
      videoCaptureDefaults: {
        resolution: { width: 640, height: 480, frameRate: 15 }
      }
    })

    // Event listeners
    room.on(RoomEvent.ConnectionStateChanged, (state: ConnectionState) => {
      switch (state) {
        case ConnectionState.Connected:
          connectionState.value = 'connected'
          emit('connected')
          break
        case ConnectionState.Reconnecting:
          connectionState.value = 'reconnecting'
          break
        case ConnectionState.Disconnected:
          connectionState.value = 'disconnected'
          hasVideo.value = false
          emit('disconnected')
          break
      }
    })

    room.on(RoomEvent.TrackSubscribed, (track, _publication, _participant) => {
      if (track.kind === Track.Kind.Video && videoRef.value) {
        track.attach(videoRef.value)
        hasVideo.value = true
      }
    })

    room.on(RoomEvent.TrackUnsubscribed, (track) => {
      if (track.kind === Track.Kind.Video) {
        track.detach()
        hasVideo.value = false
      }
    })

    room.on(RoomEvent.ParticipantConnected, () => {
      participantCount.value = room?.remoteParticipants?.size ?? 0
      attachExistingVideoTracks()
    })

    room.on(RoomEvent.ParticipantDisconnected, () => {
      participantCount.value = room?.remoteParticipants?.size ?? 0
    })

    // Connect to LiveKit server
    await room.connect(tokenResp.wsUrl, tokenResp.token)
    participantCount.value = room.remoteParticipants.size

    // Attach any existing video tracks
    attachExistingVideoTracks()
  } catch (err: any) {
    connectionState.value = 'error'
    errorMessage.value = err.message || 'Connection failed'
    emit('error', errorMessage.value)
  }
}

function attachExistingVideoTracks() {
  if (!room) return
  room.remoteParticipants.forEach((participant: RemoteParticipant) => {
    participant.trackPublications.forEach((publication: RemoteTrackPublication) => {
      if (publication.kind === Track.Kind.Video && !publication.isSubscribed) {
        publication.setSubscribed(true)
      }
      if (publication.track && publication.track.kind === Track.Kind.Video && videoRef.value) {
        publication.track.attach(videoRef.value)
        hasVideo.value = true
      }
    })
  })
}

// --- Disconnect ---
function disconnect() {
  if (room) {
    room.disconnect()
    room = null
  }
  connectionState.value = 'idle'
  hasVideo.value = false
  participantCount.value = 0
}

// --- Lifecycle ---
onMounted(() => {
  connect()
})

onUnmounted(() => {
  disconnect()
})

// --- Expose video element for evidence capture ---
defineExpose({ videoRef })

// --- Connection state label ---
const stateLabel = computed(() => {
  switch (connectionState.value) {
    case 'idle': return 'Ожидание'
    case 'connecting': return 'Подключение...'
    case 'connected': return hasVideo.value ? 'LIVE' : 'Ожидание видео'
    case 'reconnecting': return 'Переподключение...'
    case 'disconnected': return 'Отключён'
    case 'error': return 'Ошибка'
    default: return ''
  }
})

const stateColor = computed(() => {
  switch (connectionState.value) {
    case 'connected': return hasVideo.value ? '#34d399' : '#fbbf24'
    case 'connecting':
    case 'reconnecting': return '#38bdf8'
    case 'error': return '#f87171'
    default: return 'var(--argus-text-dimmed)'
  }
})
</script>

<template>
  <div
    class="relative w-full h-full overflow-hidden"
    style="background: var(--argus-bg-deep);"
  >
    <!-- Actual video element (hidden when no stream) -->
    <video
      ref="videoRef"
      class="absolute inset-0 w-full h-full object-cover"
      :class="{ 'opacity-0': !hasVideo }"
      autoplay
      playsinline
      muted
    />

    <!-- Placeholder / Loading state (shown when no video) -->
    <div
      v-if="!hasVideo"
      class="absolute inset-0 flex items-center justify-center"
    >
      <div class="flex flex-col items-center gap-2">
        <!-- Connecting spinner -->
        <div
          v-if="connectionState === 'connecting' || connectionState === 'reconnecting'"
          class="relative"
        >
          <div
            class="size-10 rounded-full border-2 animate-spin"
            style="border-color: var(--argus-border); border-top-color: var(--argus-accent);"
          />
          <UIcon
            name="i-lucide-video"
            class="absolute inset-0 m-auto size-4"
            style="color: var(--argus-accent);"
          />
        </div>

        <!-- Idle / waiting -->
        <div
          v-else-if="connectionState === 'connected'"
          class="flex flex-col items-center gap-1 opacity-40"
        >
          <UIcon
            name="i-lucide-video"
            class="size-8"
            style="color: var(--argus-text-dimmed);"
          />
          <span
            class="text-[9px] font-medium"
            style="color: var(--argus-text-dimmed);"
          >Ожидание камеры студента</span>
        </div>

        <!-- Error state -->
        <div
          v-else-if="connectionState === 'error'"
          class="flex flex-col items-center gap-1 opacity-60"
        >
          <UIcon
            name="i-lucide-video-off"
            class="size-8"
            style="color: var(--argus-error);"
          />
          <span
            class="text-[9px] font-medium"
            style="color: var(--argus-error);"
          >{{ errorMessage || 'Нет соединения' }}</span>
          <button
            class="mt-1 px-2 py-0.5 rounded text-[8px] font-bold cursor-pointer transition-all"
            style="background: var(--argus-accent); color: #fff;"
            @click="disconnect(); connect()"
          >
            Повторить
          </button>
        </div>

        <!-- Disconnected / Idle -->
        <div
          v-else
          class="flex flex-col items-center gap-1 opacity-30"
        >
          <UIcon
            name="i-lucide-video"
            class="size-8"
            style="color: var(--argus-text-dimmed);"
          />
          <span
            class="text-[9px] font-medium"
            style="color: var(--argus-text-dimmed);"
          >ВИДЕОПОТОК</span>
        </div>
      </div>
    </div>

    <!-- Connection status badge (top-left, only in compact mode) -->
    <div
      v-if="compact && connectionState !== 'idle'"
      class="absolute top-1.5 left-1.5 flex items-center gap-1 px-1.5 py-0.5 rounded-md text-[8px] font-bold backdrop-blur-sm z-10"
      :style="{
        background: `${stateColor}20`,
        color: stateColor,
        border: `1px solid ${stateColor}30`
      }"
    >
      <span
        v-if="connectionState === 'connected' && hasVideo"
        class="size-1.5 rounded-full animate-pulse"
        :style="{ background: stateColor }"
      />
      {{ stateLabel }}
    </div>
  </div>
</template>
