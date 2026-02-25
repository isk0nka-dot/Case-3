<script setup lang="ts">
const props = defineProps<{
  tabs: Array<{ label: string; language: string; code: string }>
}>()

const activeTab = ref(0)
const copied = ref(false)

function copyCode() {
  const code = props.tabs[activeTab.value]?.code
  if (code) {
    navigator.clipboard.writeText(code)
    copied.value = true
    setTimeout(() => { copied.value = false }, 2000)
  }
}
</script>

<template>
  <div
    class="rounded-xl border overflow-hidden my-6"
    :style="{ background: 'var(--argus-bg-elevated)', borderColor: 'var(--argus-border)' }"
  >
    <!-- Tab bar -->
    <div
      class="flex items-center justify-between px-4 py-2 border-b"
      :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
    >
      <div class="flex gap-1">
        <button
          v-for="(tab, i) in tabs"
          :key="tab.label"
          class="px-3 py-1.5 rounded-md text-xs font-medium transition-all cursor-pointer"
          :style="activeTab === i
            ? { background: 'rgba(56,189,248,0.1)', color: 'var(--argus-accent)' }
            : { color: 'var(--argus-text-dimmed)' }"
          @click="activeTab = i"
        >
          {{ tab.label }}
        </button>
      </div>
      <button
        class="flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium transition-all cursor-pointer"
        :style="{ color: copied ? 'var(--argus-success)' : 'var(--argus-text-dimmed)' }"
        @click="copyCode"
      >
        <UIcon :name="copied ? 'i-lucide-check' : 'i-lucide-copy'" class="size-3.5" />
        {{ copied ? 'Copied!' : 'Copy' }}
      </button>
    </div>
    <!-- Code content -->
    <div class="overflow-x-auto">
      <pre class="p-4 text-[13px] leading-relaxed"><code :style="{ color: 'var(--argus-text-muted)', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace' }">{{ tabs[activeTab]?.code }}</code></pre>
    </div>
  </div>
</template>
