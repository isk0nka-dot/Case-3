<script setup lang="ts">
interface Props {
  size?: number
  animated?: boolean
  showText?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  size: 32,
  animated: false,
  showText: false
})

const colorMode = useColorMode()
const isDark = computed(() => colorMode.value === 'dark')

// The logo viewBox is 257x300 for white.svg — we normalise to a square
// by using viewBox="0 0 257 300" and letting the SVG fit within the given size
</script>

<template>
  <div class="inline-flex items-center gap-2 shrink-0 argus-logo-wrapper" :class="{ 'argus-logo-animated': props.animated }" :data-size="props.size <= 40 ? 'sm' : undefined">
    <div class="relative shrink-0" :style="{ width: `${props.size}px`, height: `${props.size}px` }">
      <!-- Animated glow ring behind the logo (only when animated) -->
      <div
        v-if="props.animated"
        class="absolute inset-0 glow-ring"
        :style="{
          borderRadius: '50%',
          background: 'transparent',
        }"
      />

      <svg
        :width="props.size"
        :height="props.size"
        viewBox="0 0 257 300"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        class="shrink-0 logo-svg"
        style="display: block;"
      >
        <defs>
          <!-- Filters for animated glow per segment -->
          <filter id="glow-blue" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="4" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
          <filter id="glow-pink" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="4" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
          <filter id="glow-green" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="4" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
          <filter id="glow-orange" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="4" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
          <filter id="glow-red" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="4" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
          <filter id="glow-purple" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="4" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
        </defs>

        <!-- === CENTER EYE SHAPE (dark for light mode, white for dark mode) === -->
        <path
          fill-rule="evenodd"
          clip-rule="evenodd"
          d="M149.995 110.769C171.666 110.769 189.226 128.334 189.226 150C189.226 171.666 171.666 189.23 149.995 189.23C128.325 189.23 110.76 171.666 110.76 150C110.76 128.334 128.325 110.769 149.995 110.769ZM149.995 60.7905C199.266 60.7905 239.205 100.729 239.205 150V239.209H214.213C205.539 239.209 197.862 234.726 193.374 227.965C180.535 235.125 165.74 239.209 149.995 239.209C100.725 239.209 60.7859 199.27 60.7859 150C60.7859 100.729 100.725 60.7905 149.995 60.7905Z"
          :fill="isDark ? '#E2E8F0' : '#1A1B23'"
          class="logo-center"
        />

        <!-- === COLORED SEGMENTS (outer arc pieces) === -->
        <!-- Purple segment (bottom-right) -->
        <path
          fill-rule="evenodd"
          clip-rule="evenodd"
          d="M154.928 300C194.409 298.727 230.046 282.206 256.124 256.125L239.208 239.209H214.217C205.543 239.209 197.865 234.726 193.382 227.969C181.887 234.374 168.832 238.314 154.928 239.074V300Z"
          fill="#8B5CF6"
          class="segment seg-purple"
        />

        <!-- Blue segment (top-right) -->
        <path
          fill-rule="evenodd"
          clip-rule="evenodd"
          d="M154.928 60.9264C177.599 62.1607 198.018 71.857 213.076 86.9189L256.124 43.8709C230.046 17.7935 194.413 1.26824 154.928 0V60.9264Z"
          fill="#2563EB"
          class="segment seg-blue"
        />

        <!-- Pink segment (top-left) -->
        <path
          fill-rule="evenodd"
          clip-rule="evenodd"
          d="M90.5023 83.5299C105.149 70.4149 124.147 62.0674 145.066 60.9264V0.00430298C107.341 1.22164 73.1372 16.3599 47.4204 40.4522L90.4981 83.5299H90.5023Z"
          fill="#F633B4"
          class="segment seg-pink"
        />

        <!-- Green segment (left) -->
        <path
          fill-rule="evenodd"
          clip-rule="evenodd"
          d="M60.9262 145.067C62.0672 124.147 70.4146 105.149 83.5339 90.5031L40.4562 47.4254C16.3681 73.1464 1.22564 107.346 0.00830078 145.067H60.9304H60.9262Z"
          fill="#10A34A"
          class="segment seg-green"
        />

        <!-- Orange segment (bottom-left-lower) -->
        <path
          fill-rule="evenodd"
          clip-rule="evenodd"
          d="M83.5299 209.493C70.4106 194.846 62.0674 175.848 60.9221 154.929H0C1.21734 192.654 16.3556 226.858 40.4479 252.575L83.5256 209.497L83.5299 209.493Z"
          fill="#E67E22"
          class="segment seg-orange"
        />

        <!-- Red segment (bottom-left-upper) -->
        <path
          fill-rule="evenodd"
          clip-rule="evenodd"
          d="M145.066 239.074C124.142 237.933 105.144 229.585 90.5023 216.466L47.4246 259.544C73.1456 283.632 107.346 298.774 145.071 299.992V239.07L145.066 239.074Z"
          fill="#E03E3E"
          class="segment seg-red"
        />
      </svg>
    </div>

    <!-- Optional text -->
    <span
      v-if="props.showText"
      class="font-semibold tracking-tight whitespace-nowrap"
      style="color: var(--argus-text);"
      :style="{ fontSize: `${Math.max(props.size * 0.45, 12)}px` }"
    >
      Argus AI
    </span>
  </div>
</template>

<style scoped>
/* =============================================
   IDLE STATE — subtle breathing on segments
   ============================================= */
.argus-logo-wrapper .segment {
  transform-origin: 128px 150px; /* center of the 257x300 viewBox */
  transition: transform 0.4s ease, filter 0.4s ease, opacity 0.4s ease;
}

/* Hover glow on segments */
.argus-logo-wrapper:hover .segment {
  filter: drop-shadow(0 0 3px currentColor);
}

/* =============================================
   ANIMATED STATE — voice-indicator pulsing
   Each segment gently scales outward and
   glows, staggered like an audio visualizer
   ============================================= */
.argus-logo-animated .seg-blue {
  animation: seg-wave-out 2.8s ease-in-out infinite 0s;
}
.argus-logo-animated .seg-pink {
  animation: seg-wave-out 2.8s ease-in-out infinite 0.45s;
}
.argus-logo-animated .seg-green {
  animation: seg-wave-out 2.8s ease-in-out infinite 0.9s;
}
.argus-logo-animated .seg-orange {
  animation: seg-wave-out 2.8s ease-in-out infinite 1.35s;
}
.argus-logo-animated .seg-red {
  animation: seg-wave-out 2.8s ease-in-out infinite 1.8s;
}
.argus-logo-animated .seg-purple {
  animation: seg-wave-out 2.8s ease-in-out infinite 2.25s;
}

/* Voice-indicator wave: segments push outward slightly and glow */
@keyframes seg-wave-out {
  0%, 100% {
    transform: scale(1);
    opacity: 0.85;
    filter: drop-shadow(0 0 0px transparent);
  }
  30% {
    transform: scale(1.06);
    opacity: 1;
    filter: drop-shadow(0 0 6px currentColor);
  }
  60% {
    transform: scale(0.97);
    opacity: 0.75;
    filter: drop-shadow(0 0 1px transparent);
  }
}

/* Animated glow ring behind */
.glow-ring {
  animation: ring-glow 3s ease-in-out infinite;
}

@keyframes ring-glow {
  0%, 100% {
    box-shadow:
      0 0 8px 2px rgba(37, 99, 235, 0.15),
      0 0 8px 2px rgba(139, 92, 246, 0.1);
  }
  33% {
    box-shadow:
      0 0 14px 4px rgba(246, 51, 180, 0.2),
      0 0 12px 3px rgba(16, 163, 74, 0.15);
  }
  66% {
    box-shadow:
      0 0 12px 3px rgba(230, 126, 34, 0.2),
      0 0 10px 3px rgba(224, 62, 62, 0.15);
  }
}

/* Subtle variant for small navbar-size logos */
.argus-logo-wrapper[data-size="sm"] .glow-ring {
  animation: ring-glow-sm 4s ease-in-out infinite;
}

.argus-logo-wrapper[data-size="sm"] .segment {
  animation-duration: 3.5s !important;
}

@keyframes ring-glow-sm {
  0%, 100% {
    box-shadow: 0 0 4px 1px rgba(37, 99, 235, 0.1);
  }
  50% {
    box-shadow: 0 0 8px 2px rgba(139, 92, 246, 0.12);
  }
}

/* Center eye stays perfectly still */
.argus-logo-animated .logo-center {
  transform-origin: 128px 150px;
}

/* Logo SVG overflow visible for glow effects */
.logo-svg {
  overflow: visible;
}
</style>
