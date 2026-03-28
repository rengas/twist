<script setup>
import { computed, ref } from 'vue'
import TaskCard from './TaskCard.vue'
import TaskModal from './TaskModal.vue'

const props = defineProps({ tasks: Array })
const emit = defineEmits(['refresh', 'open-chat'])

const selectedTask = ref(null)
const showArchive = ref(false)

const columns = [
  { key: 'prompt',   label: 'Prompt',   color: 'from-violet-500/20 to-violet-600/10', badge: 'bg-violet-500/30 text-violet-300', dot: 'bg-violet-400' },
  { key: 'spec',     label: 'Spec',     color: 'from-amber-500/20 to-amber-600/10',   badge: 'bg-amber-500/30 text-amber-300',   dot: 'bg-amber-400'   },
  { key: 'code',     label: 'Code',     color: 'from-sky-500/20 to-sky-600/10',       badge: 'bg-sky-500/30 text-sky-300',       dot: 'bg-sky-400'     },
  { key: 'review',   label: 'Review',   color: 'from-orange-500/20 to-orange-600/10', badge: 'bg-orange-500/30 text-orange-300', dot: 'bg-orange-400'  },
  { key: 'done',     label: 'Done',     color: 'from-emerald-500/20 to-emerald-600/10', badge: 'bg-emerald-500/30 text-emerald-300', dot: 'bg-emerald-400' },
  { key: 'failed',   label: 'Failed',   color: 'from-red-500/20 to-red-600/10',       badge: 'bg-red-500/30 text-red-300',       dot: 'bg-red-400'     },
  { key: 'archived', label: 'Archive',  color: 'from-slate-500/20 to-slate-600/10',   badge: 'bg-slate-500/30 text-slate-300',   dot: 'bg-slate-400'   },
]

const visibleColumns = computed(() =>
  showArchive.value ? columns : columns.filter(c => c.key !== 'archived')
)

const archivedCount = computed(() =>
  (props.tasks || []).filter(t => t.status === 'archived').length
)

function tasksForColumn(key) {
  return (props.tasks || []).filter(t => t.status === key)
}

function openTask(task) {
  selectedTask.value = task
}

function onModalClose() {
  selectedTask.value = null
  emit('refresh')
}
</script>

<template>
  <div class="flex flex-col h-full">
    <!-- Archive toggle -->
    <div class="flex items-center justify-end px-4 pt-2 select-none">
      <button @click="showArchive = !showArchive"
              class="flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-xs transition-colors"
              :class="showArchive ? 'bg-slate-600 text-slate-200' : 'bg-slate-800 text-slate-400 hover:text-slate-300'">
        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4"/>
        </svg>
        Archive
        <span v-if="archivedCount > 0"
              class="px-1.5 py-0.5 rounded-full text-[10px] font-medium bg-slate-500/30 text-slate-300">
          {{ archivedCount }}
        </span>
      </button>
    </div>

    <div class="flex gap-2 sm:gap-3 px-3 sm:px-4 py-3 sm:py-4 overflow-x-auto flex-1 snap-x snap-mandatory sm:snap-none">
      <div v-for="col in visibleColumns" :key="col.key"
           class="flex flex-col min-w-[160px] w-52 flex-1 flex-shrink snap-start rounded-xl bg-slate-800/50 border border-slate-700/40 backdrop-blur-sm">
        <!-- Column Header -->
        <div class="flex items-center gap-2 px-3 pt-3 pb-2.5 select-none">
          <div class="w-2 h-2 rounded-full" :class="col.dot"></div>
          <span class="text-xs font-semibold uppercase tracking-wider text-slate-400">{{ col.label }}</span>
          <span class="ml-auto text-xs px-1.5 py-0.5 rounded-full font-medium" :class="col.badge">
            {{ tasksForColumn(col.key).length }}
          </span>
        </div>
        <div class="w-full h-px bg-gradient-to-r" :class="col.color"></div>

        <!-- Task Cards -->
        <div class="flex flex-col gap-2 p-2 overflow-y-auto flex-1">
          <TaskCard v-for="task in tasksForColumn(col.key)" :key="task.id"
                    :task="task" :col-key="col.key"
                    @click="openTask(task)"
                    @open-chat="(id) => emit('open-chat', id)" />
          <div v-if="tasksForColumn(col.key).length === 0"
               class="flex-1 flex items-center justify-center text-slate-600 text-xs py-6">
            Empty
          </div>
        </div>
      </div>
    </div>
  </div>

  <TaskModal v-if="selectedTask" :task="selectedTask" @close="onModalClose"
             @open-chat="(id) => { selectedTask = null; emit('open-chat', id) }" />
</template>
