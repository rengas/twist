<script setup>
import { computed, ref, watch } from 'vue'
import { marked } from 'marked'
import { ApproveTask, DeleteTask } from '../../wailsjs/go/pkg/App'

const props = defineProps({ task: Object })
const emit = defineEmits(['close'])

const loading = ref(false)
const error = ref('')
const isSpecExpanded = ref(false)

watch(() => props.task?.id, () => {
  isSpecExpanded.value = false
})

const renderedSpec = computed(() => {
  if (!props.task.spec) return ''
  return marked.parse(props.task.spec)
})

const canApprove = computed(() => {
  const s = props.task.status
  return ['prompt', 'spec', 'code', 'review', 'failed'].includes(s) && !props.task.approved
})

const approveLabel = computed(() => {
  const s = props.task.status
  if (s === 'prompt') return 'Approve — Generate Spec'
  if (s === 'spec')   return 'Approve Spec → Implement'
  if (s === 'code')   return 'Approve — Start Build'
  if (s === 'review') return 'Approve PR → Done'
  if (s === 'failed') return 'Retry Implementation'
  return 'Approve'
})

const statusColor = computed(() => ({
  prompt:   'bg-violet-500/30 text-violet-300',
  spec:     'bg-amber-500/30 text-amber-300',
  code:     'bg-sky-500/30 text-sky-300',
  review:   'bg-orange-500/30 text-orange-300',
  done:     'bg-emerald-500/30 text-emerald-300',
  failed:   'bg-red-500/30 text-red-300',
}[props.task.status] || 'bg-slate-500/30 text-slate-300'))

async function approve() {
  loading.value = true
  error.value = ''
  try {
    await ApproveTask(props.task.id)
    emit('close')
  } catch (e) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
}

async function deleteTask() {
  if (!confirm(`Delete task #${props.task.id}?`)) return
  loading.value = true
  try {
    await DeleteTask(props.task.id)
    emit('close')
  } catch (e) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <!-- Backdrop -->
  <div class="fixed inset-0 z-50 flex items-center justify-center p-6 bg-black/60 backdrop-blur-sm"
       @click.self="emit('close')">
    <!-- Modal -->
    <div class="relative w-full max-w-2xl max-h-[85vh] flex flex-col rounded-2xl
                bg-slate-800 border border-slate-700/60 shadow-2xl shadow-black/50">

      <!-- Header -->
      <div class="flex items-start gap-3 px-6 pt-5 pb-4 border-b border-slate-700/50">
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2 mb-1">
            <span class="text-[10px] font-mono text-slate-500">#{{ task.id }}</span>
            <span class="text-[10px] px-1.5 py-0.5 rounded-full font-semibold uppercase tracking-wide" :class="statusColor">
              {{ task.status }}
            </span>
            <span v-if="task.approved" class="text-[10px] px-1.5 py-0.5 rounded-full bg-emerald-500/20 text-emerald-400 font-semibold">
              Approved
            </span>
          </div>
          <h2 class="text-base font-semibold text-slate-100 leading-snug">{{ task.title }}</h2>
          <p v-if="task.branch" class="text-xs text-sky-400 font-mono mt-0.5">{{ task.branch }}</p>
          <a v-if="task.pr_url" :href="task.pr_url" target="_blank"
             class="text-xs text-blue-400 hover:text-blue-300 underline mt-0.5 inline-block">
            {{ task.pr_url }}
          </a>
        </div>
        <button @click="emit('close')"
                class="text-slate-500 hover:text-slate-300 transition-colors mt-0.5 flex-shrink-0">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
          </svg>
        </button>
      </div>

      <!-- Scrollable content -->
      <div class="overflow-y-auto flex-1 px-6 py-4 space-y-4">
        <!-- Prompt section -->
        <div>
          <h3 class="text-[10px] uppercase tracking-widest font-semibold text-slate-500 mb-1.5">Prompt</h3>
          <p class="text-sm text-slate-300 bg-slate-900/50 rounded-lg px-3 py-2.5 leading-relaxed">{{ task.prompt }}</p>
        </div>

        <!-- Spec section (collapsible) -->
        <div v-if="task.spec" class="spec-section">
          <button
            class="spec-toggle"
            :aria-expanded="isSpecExpanded"
            @click="isSpecExpanded = !isSpecExpanded"
          >
            <svg
              class="chevron"
              :class="{ 'chevron-expanded': isSpecExpanded }"
              width="16" height="16" viewBox="0 0 16 16"
              fill="currentColor"
            >
              <path d="M6 4l4 4-4 4" />
            </svg>
            <span class="spec-toggle-label">Spec</span>
          </button>

          <div
            class="spec-content"
            :class="{ 'spec-content-expanded': isSpecExpanded }"
          >
            <div class="prose text-sm bg-slate-900/50 rounded-lg px-4 py-3" v-html="renderedSpec"></div>
          </div>
        </div>
        <div v-else-if="task.status !== 'prompt'" class="text-sm text-slate-600 italic">
          No spec yet — agent will generate it when approved.
        </div>
      </div>

      <!-- Footer actions -->
      <div class="flex items-center justify-between px-6 py-4 border-t border-slate-700/50">
        <button @click="deleteTask" :disabled="loading"
                class="text-xs text-red-400 hover:text-red-300 transition-colors disabled:opacity-50">
          Delete task
        </button>

        <div class="flex items-center gap-3">
          <p v-if="error" class="text-xs text-red-400">{{ error }}</p>

          <button @click="emit('close')"
                  class="px-4 py-2 text-xs rounded-lg bg-slate-700 hover:bg-slate-600 text-slate-300 transition-colors">
            Close
          </button>

          <button v-if="canApprove" @click="approve" :disabled="loading"
                  class="flex items-center gap-2 px-4 py-2 text-xs rounded-lg font-semibold
                         bg-violet-600 hover:bg-violet-500 text-white transition-colors disabled:opacity-50">
            <svg v-if="loading" class="w-3.5 h-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4l3-3-3-3v4a8 8 0 000 16v-4l-3 3 3 3v-4a8 8 0 01-8-8z"/>
            </svg>
            <svg v-else class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
            </svg>
            {{ approveLabel }}
          </button>

          <span v-else-if="task.approved && task.status !== 'done'"
                class="text-xs text-emerald-400">
            Agent is working…
          </span>
          <span v-else-if="task.status === 'done'"
                class="text-xs text-emerald-400">
            Done
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.spec-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  background: none;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 6px;
  padding: 8px 12px;
  color: inherit;
  cursor: pointer;
  font-size: 0.95rem;
  font-weight: 600;
}

.spec-toggle:hover {
  background: rgba(255, 255, 255, 0.05);
}

.chevron {
  transition: transform 0.2s ease;
  flex-shrink: 0;
}

.chevron-expanded {
  transform: rotate(90deg);
}

.spec-content {
  max-height: 0;
  overflow: hidden;
  transition: max-height 0.25s ease;
}

.spec-content-expanded {
  max-height: 2000px;
}
</style>
