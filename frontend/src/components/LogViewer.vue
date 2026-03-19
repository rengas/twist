<script setup>
import { ref, watch, nextTick } from 'vue'

const props = defineProps({ logs: Array })
const container = ref(null)
const collapsed = ref(false)

watch(() => props.logs.length, async () => {
  await nextTick()
  if (container.value) {
    container.value.scrollTop = container.value.scrollHeight
  }
})

function lineColor(line) {
  if (line.includes('[ERROR]') || line.includes('[FAILED]')) return 'text-red-400'
  if (line.includes('[CLAUDE ERR]')) return 'text-orange-400'
  if (line.includes('[CLAUDE]')) return 'text-emerald-400'
  if (line.includes('[GIT]')) return 'text-sky-400'
  if (line.includes('[PR]') || line.includes('[PR RAISED]')) return 'text-violet-400'
  if (line.includes('[HEARTBEAT]')) return 'text-slate-600'
  if (line.includes('[SPEC') || line.includes('[CODE') || line.includes('[REVIEW')) return 'text-amber-400'
  if (line.includes('[COMPLETE]')) return 'text-emerald-300'
  if (line.includes('[CONFIG]')) return 'text-slate-500'
  return 'text-slate-400'
}
</script>

<template>
  <div class="flex-shrink-0 border-t border-slate-700/50 bg-slate-900/80">
    <!-- Log header bar -->
    <div class="flex items-center justify-between px-4 py-1.5 cursor-pointer select-none"
         @click="collapsed = !collapsed">
      <div class="flex items-center gap-2">
        <svg class="w-3.5 h-3.5 text-slate-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"/>
        </svg>
        <span class="text-[10px] uppercase tracking-widest font-semibold text-slate-500">Agent Log</span>
        <span class="text-[10px] text-slate-600">{{ logs.length }} lines</span>
      </div>
      <svg class="w-3.5 h-3.5 text-slate-600 transition-transform" :class="collapsed ? 'rotate-180' : ''"
           fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/>
      </svg>
    </div>

    <!-- Log lines -->
    <div v-if="!collapsed" ref="container"
         class="overflow-y-auto font-mono text-[11px] leading-relaxed px-4 pb-2"
         style="max-height: 140px">
      <div v-if="logs.length === 0" class="text-slate-700 py-1">No log output yet…</div>
      <div v-for="(line, i) in logs" :key="i" :class="lineColor(line)">{{ line }}</div>
    </div>
  </div>
</template>
