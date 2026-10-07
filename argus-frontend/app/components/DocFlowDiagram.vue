<script setup lang="ts">
defineEmits<{
  navigate: [sectionId: string]
}>()

const steps = [
  {
    number: 1,
    icon: 'i-lucide-key-round',
    title: 'Authenticate',
    description: 'Obtain a JWT token using your API key credentials.',
    sectionId: 'authentication',
    color: 'var(--argus-accent)'
  },
  {
    number: 2,
    icon: 'i-lucide-plus-circle',
    title: 'Create Session',
    description: 'POST to create a proctoring session with exam & student IDs.',
    sectionId: 'session-flow',
    color: 'var(--argus-success)'
  },
  {
    number: 3,
    icon: 'i-lucide-monitor-play',
    title: 'Launch UI',
    description: 'Initialize the widget. PreExamCheck validates hardware & identity.',
    sectionId: 'widget-integration',
    color: 'var(--argus-brand-purple)'
  },
  {
    number: 4,
    icon: 'i-lucide-webhook',
    title: 'Receive Webhook',
    description: 'Get real-time events: violations, session end, integrity scores.',
    sectionId: 'webhooks',
    color: 'var(--argus-warning)'
  }
]
</script>

<template>
  <div class="my-8">
    <!-- Desktop: horizontal flow -->
    <div class="hidden md:flex items-start justify-between gap-2">
      <template
        v-for="(step, i) in steps"
        :key="step.number"
      >
        <!-- Step card -->
        <button
          class="flex-1 rounded-xl border p-5 transition-all cursor-pointer group"
          :style="{
            background: 'var(--argus-bg-card)',
            borderColor: 'var(--argus-border)'
          }"
          @mouseenter="($event.currentTarget as HTMLElement).style.borderColor = step.color"
          @mouseleave="($event.currentTarget as HTMLElement).style.borderColor = 'var(--argus-border)'"
          @click="$emit('navigate', step.sectionId)"
        >
          <div class="flex items-center gap-3 mb-3">
            <div
              class="flex items-center justify-center size-10 rounded-lg shrink-0"
              :style="{ background: `${step.color}15`, border: `1px solid ${step.color}30` }"
            >
              <UIcon
                :name="step.icon"
                class="size-5"
                :style="{ color: step.color }"
              />
            </div>
            <span
              class="text-[11px] font-bold uppercase tracking-wider"
              :style="{ color: step.color }"
            >
              Step {{ step.number }}
            </span>
          </div>
          <h4
            class="text-sm font-semibold mb-1"
            style="color: var(--argus-text);"
          >
            {{ step.title }}
          </h4>
          <p
            class="text-xs leading-relaxed"
            style="color: var(--argus-text-dimmed);"
          >
            {{ step.description }}
          </p>
        </button>

        <!-- Arrow connector -->
        <div
          v-if="i < steps.length - 1"
          class="flex items-center justify-center shrink-0 pt-8"
        >
          <UIcon
            name="i-lucide-arrow-right"
            class="size-5"
            style="color: var(--argus-text-dimmed);"
          />
        </div>
      </template>
    </div>

    <!-- Mobile: vertical flow -->
    <div class="md:hidden space-y-3">
      <template
        v-for="(step, i) in steps"
        :key="step.number"
      >
        <button
          class="w-full rounded-xl border p-4 text-left transition-all cursor-pointer"
          :style="{
            background: 'var(--argus-bg-card)',
            borderColor: 'var(--argus-border)'
          }"
          @click="$emit('navigate', step.sectionId)"
        >
          <div class="flex items-center gap-3">
            <div
              class="flex items-center justify-center size-9 rounded-lg shrink-0"
              :style="{ background: `${step.color}15`, border: `1px solid ${step.color}30` }"
            >
              <UIcon
                :name="step.icon"
                class="size-4"
                :style="{ color: step.color }"
              />
            </div>
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 mb-0.5">
                <span
                  class="text-[10px] font-bold uppercase tracking-wider"
                  :style="{ color: step.color }"
                >
                  Step {{ step.number }}
                </span>
                <span
                  class="text-sm font-semibold"
                  style="color: var(--argus-text);"
                >
                  {{ step.title }}
                </span>
              </div>
              <p
                class="text-xs"
                style="color: var(--argus-text-dimmed);"
              >
                {{ step.description }}
              </p>
            </div>
          </div>
        </button>

        <!-- Vertical arrow -->
        <div
          v-if="i < steps.length - 1"
          class="flex justify-center"
        >
          <UIcon
            name="i-lucide-arrow-down"
            class="size-4"
            style="color: var(--argus-text-dimmed);"
          />
        </div>
      </template>
    </div>
  </div>
</template>
