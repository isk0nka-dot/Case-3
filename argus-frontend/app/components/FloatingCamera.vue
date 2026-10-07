<script setup lang="ts">
// =============================================================================
// FloatingCamera — Draggable PIP Camera Window
// =============================================================================
//
// Appears after PreExamCheck verification. Displays the live camera feed
// in a small floating window (top-right corner). Draggable, minimizable,
// clamped to viewport bounds.
//
// Reuses the MediaStream from PreExamCheck — no new getUserMedia call.
// =============================================================================

import { ref, watch, onMounted, onUnmounted } from 'vue'

const props = defineProps<{
  stream: MediaStream | null
  isVisible: boolean
}>()

// ---------------------------------------------------------------------------
// Refs
// ---------------------------------------------------------------------------

const videoRef = ref<HTMLVideoElement | null>(null)
const minimized = ref(false)
const position = ref({ x: 0, y: 20 })
const isDragging = ref(false)
const dragOffset = ref({ x: 0, y: 0 })

const PIP_WIDTH = 240
const PIP_HEIGHT = 180
const PIP_MIN_SIZE = 56
const MARGIN = 16

// ---------------------------------------------------------------------------
// Video stream binding
// ---------------------------------------------------------------------------

function attachStream(): void {
  if (videoRef.value && props.stream) {
    videoRef.value.srcObject = props.stream
    void videoRef.value.play().catch(() => {
      // Autoplay may fail on some browsers; that's OK — muted video autoplay is allowed
    })
  }
}

watch(() => props.stream, () => attachStream())
watch(() => props.isVisible, (v) => {
  if (v) {
    // Position in top-right corner
    position.value.x = window.innerWidth - PIP_WIDTH - MARGIN
    requestAnimationFrame(() => attachStream())
  }
})

onMounted(() => {
  position.value.x = window.innerWidth - PIP_WIDTH - MARGIN
  attachStream()
})

// ---------------------------------------------------------------------------
// Drag logic
// ---------------------------------------------------------------------------

function startDrag(e: MouseEvent): void {
  // Don't start drag from buttons
  if ((e.target as HTMLElement).closest('button')) return

  isDragging.value = true
  dragOffset.value = {
    x: e.clientX - position.value.x,
    y: e.clientY - position.value.y
  }

  document.addEventListener('mousemove', onDrag)
  document.addEventListener('mouseup', stopDrag)
  e.preventDefault()
}

function onDrag(e: MouseEvent): void {
  if (!isDragging.value) return

  const w = minimized.value ? PIP_MIN_SIZE : PIP_WIDTH
  const h = minimized.value ? PIP_MIN_SIZE : PIP_HEIGHT

  position.value = {
    x: Math.max(0, Math.min(window.innerWidth - w, e.clientX - dragOffset.value.x)),
    y: Math.max(0, Math.min(window.innerHeight - h, e.clientY - dragOffset.value.y))
  }
}

function stopDrag(): void {
  isDragging.value = false
  document.removeEventListener('mousemove', onDrag)
  document.removeEventListener('mouseup', stopDrag)
}

onUnmounted(() => {
  document.removeEventListener('mousemove', onDrag)
  document.removeEventListener('mouseup', stopDrag)
})

function toggleMinimize(): void {
  minimized.value = !minimized.value
  // Re-clamp position after resize
  const w = minimized.value ? PIP_MIN_SIZE : PIP_WIDTH
  const h = minimized.value ? PIP_MIN_SIZE : PIP_HEIGHT
  position.value = {
    x: Math.min(position.value.x, window.innerWidth - w),
    y: Math.min(position.value.y, window.innerHeight - h)
  }
}
</script>

<template>
  <Teleport to="body">
    <Transition name="pip">
      <div
        v-if="isVisible && stream"
        class="fixed z-[200] overflow-hidden select-none"
        :class="[
          isDragging ? 'cursor-grabbing' : 'cursor-grab',
          minimized ? 'rounded-full' : 'rounded-xl'
        ]"
        :style="{
          top: `${position.y}px`,
          left: `${position.x}px`,
          width: minimized ? `${PIP_MIN_SIZE}px` : `${PIP_WIDTH}px`,
          height: minimized ? `${PIP_MIN_SIZE}px` : `${PIP_HEIGHT}px`,
          border: `1px solid var(--argus-border, #1E293B)`,
          background: 'var(--argus-bg-deep, #0B0F14)',
          boxShadow: '0 8px 32px rgba(0, 0, 0, 0.5), 0 2px 8px rgba(0, 0, 0, 0.3)',
          transition: isDragging ? 'none' : 'width 0.3s ease, height 0.3s ease, border-radius 0.3s ease'
        }"
        @mousedown="startDrag"
      >
        <!-- Video feed -->
        <video
          ref="videoRef"
          autoplay
          playsinline
          muted
          class="w-full h-full object-cover mirror"
          :class="{ 'rounded-full': minimized }"
        />

        <!-- Controls overlay (hidden when minimized) -->
        <template v-if="!minimized">
          <!-- Drag handle -->
          <div
            class="absolute top-1.5 left-1/2 -translate-x-1/2 w-8 h-1 rounded-full pointer-events-none"
            style="background: rgba(255, 255, 255, 0.2)"
          />

          <!-- Minimize button -->
          <button
            class="absolute top-1.5 right-1.5 size-6 rounded-md flex items-center justify-center transition-colors"
            style="background: rgba(0, 0, 0, 0.5)"
            @click.stop="toggleMinimize"
          >
            <UIcon
              name="i-lucide-minimize-2"
              class="size-3 text-white/60"
            />
          </button>
        </template>

        <!-- Expand indicator when minimized -->
        <button
          v-if="minimized"
          class="absolute inset-0 flex items-center justify-center rounded-full"
          style="background: rgba(0, 0, 0, 0.3)"
          @click.stop="toggleMinimize"
        >
          <UIcon
            name="i-lucide-maximize-2"
            class="size-4 text-white/80"
          />
        </button>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.mirror {
  transform: scaleX(-1);
}

.pip-enter-active {
  transition: all 0.5s cubic-bezier(0.16, 1, 0.3, 1);
}
.pip-leave-active {
  transition: all 0.3s ease;
}
.pip-enter-from {
  opacity: 0;
  transform: scale(0.5) translate(100px, -50px);
}
.pip-leave-to {
  opacity: 0;
  transform: scale(0.5);
}
</style>
