<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'
import { LoadTasks, GetWorkDir } from '../wailsjs/go/pkg/App'
import KanbanBoard from './components/KanbanBoard.vue'
import LogViewer from './components/LogViewer.vue'
import AddTaskModal from './components/AddTaskModal.vue'
import SettingsModal from './components/SettingsModal.vue'

const tasks = ref([])
const logs = ref([])
const workDir = ref('')
const showAddModal = ref(false)
const showSettings = ref(false)
const agentRunning = ref(false)

async function refresh() {
  tasks.value = await LoadTasks()
  agentRunning.value = tasks.value.some(
    t => t.approved && (t.status === 'prompt' || t.status === 'code' || t.status === 'done')
  )
}

async function onSettingsSaved() {
  workDir.value = await GetWorkDir()
  await refresh()
}

onMounted(async () => {
  workDir.value = await GetWorkDir()
  await refresh()

  EventsOn('tasks:updated', (updated) => {
    tasks.value = updated
    agentRunning.value = updated.some(
      t => t.approved && (t.status === 'prompt' || t.status === 'code' || t.status === 'done')
    )
  })

  EventsOn('log', (line) => {
    logs.value.push(line)
    if (logs.value.length > 500) logs.value.shift()
  })
})

onUnmounted(() => {
  EventsOff('tasks:updated')
  EventsOff('log')
})
</script>

<template>
  <div class="flex flex-col h-screen bg-slate-900 text-slate-200 select-none">
    <!-- Header -->
    <header class="grid grid-cols-3 items-center px-6 py-3 bg-slate-900 border-b border-slate-700/50 flex-shrink-0"
            style="--wails-draggable: drag">
      <!-- Left spacer -->
      <div></div>

      <!-- Center: Logo -->
      <div class="flex items-center justify-center gap-3">
        <div class="w-7 h-7 rounded-lg bg-gradient-to-br from-violet-500 to-indigo-600 flex items-center justify-center text-xs font-bold">
          T
        </div>
        <span class="font-semibold text-slate-100 text-sm tracking-wide">twist</span>
        <div class="flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs"
             :class="agentRunning ? 'bg-emerald-500/20 text-emerald-400' : 'bg-slate-700 text-slate-400'">
          <span class="w-1.5 h-1.5 rounded-full inline-block"
                :class="agentRunning ? 'bg-emerald-400 animate-pulse' : 'bg-slate-500'"></span>
          {{ agentRunning ? 'Agent running' : 'Idle' }}
        </div>
      </div>

      <!-- Right: Actions -->
      <div class="flex items-center justify-end gap-3" style="--wails-draggable: no-drag">
        <div class="flex items-center gap-1.5 text-xs text-slate-500 px-2 py-1"
             :title="workDir">
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M3 7a2 2 0 012-2h3.586a1 1 0 01.707.293L10.414 6.5A1 1 0 0011.12 7H19a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V7z"/>
          </svg>
          <span class="max-w-[200px] truncate">{{ workDir.split('/').pop() || workDir }}</span>
        </div>
        <button @click="showSettings = true"
                class="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-700 transition-colors"
                title="Settings">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/>
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
          </svg>
        </button>
        <button @click="showAddModal = true"
                class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-violet-600 hover:bg-violet-500 text-white text-xs font-medium transition-colors">
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/>
          </svg>
          New Task
        </button>
      </div>
    </header>

    <!-- Kanban Board -->
    <KanbanBoard :tasks="tasks" @refresh="refresh" class="flex-1 min-h-0" />

    <!-- Log Viewer -->
    <LogViewer :logs="logs" />
  </div>

  <!-- Modals -->
  <AddTaskModal v-if="showAddModal" @close="showAddModal = false" @created="refresh" />
  <SettingsModal v-model="showSettings" @saved="onSettingsSaved" />
</template>
