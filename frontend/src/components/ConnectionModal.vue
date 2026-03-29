<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { ConnectDB } from '../../wailsjs/go/pkg/App'

const props = defineProps({
  initialUrl: { type: String, default: '' },
})
const emit = defineEmits(['connected'])

const dbUrl = ref(props.initialUrl)
const errorMsg = ref('')
const connecting = ref(false)

// Migration progress state
const migrating = ref(false)
const migrationCurrent = ref(0)
const migrationTotal = ref(0)
const migrationDesc = ref('')

onMounted(() => {
  EventsOn('migration:status', (status) => {
    migrating.value = status.running
    migrationCurrent.value = status.current
    migrationTotal.value = status.total
    migrationDesc.value = status.description
  })
})

onUnmounted(() => {
  EventsOff('migration:status')
})

const migrationPercent = () => {
  if (migrationTotal.value <= 0) return 0
  return Math.round((migrationCurrent.value / migrationTotal.value) * 100)
}

async function connect() {
  if (!dbUrl.value.trim()) {
    errorMsg.value = 'Please enter a PostgreSQL connection URL.'
    return
  }
  connecting.value = true
  errorMsg.value = ''
  try {
    await ConnectDB(dbUrl.value.trim())
    emit('connected')
  } catch (e) {
    errorMsg.value = String(e)
    migrating.value = false
  } finally {
    connecting.value = false
  }
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-end sm:items-center justify-center p-0 sm:p-6 bg-slate-900">
    <div class="w-full sm:max-w-lg rounded-t-2xl sm:rounded-2xl bg-slate-800 border border-slate-700/60 shadow-2xl shadow-black/50">

      <!-- Header -->
      <div class="flex flex-col items-center pt-6 sm:pt-8 pb-3 sm:pb-4 px-4 sm:px-6">
        <div class="w-12 h-12 rounded-xl bg-gradient-to-br from-violet-500 to-indigo-600 flex items-center justify-center text-lg font-bold mb-4">
          T
        </div>
        <h1 class="text-lg font-semibold text-slate-100">Connect to Database</h1>
        <p class="text-xs text-slate-500 mt-1 text-center">
          twist requires a PostgreSQL database to persist tasks and state.
        </p>
      </div>

      <!-- Body -->
      <div class="px-4 sm:px-6 py-3 sm:py-4 space-y-4">
        <div class="space-y-1.5">
          <label class="block text-xs font-medium text-slate-400">PostgreSQL URL</label>
          <input v-model="dbUrl"
                 type="text"
                 :disabled="migrating"
                 placeholder="postgres://user:pass@localhost:5432/twist?sslmode=disable"
                 @keydown.enter="connect"
                 class="w-full bg-slate-900/60 border border-slate-700 rounded-lg px-3 py-2.5
                        text-sm text-slate-200 placeholder-slate-600 focus:outline-none
                        focus:border-violet-500 focus:ring-1 focus:ring-violet-500/50 transition-colors
                        font-mono disabled:opacity-50" />
          <p class="text-[11px] text-slate-500">
            Example: postgres://user:password@localhost:5432/twist?sslmode=disable
          </p>
        </div>

        <!-- Migration progress bar -->
        <div v-if="migrating" class="space-y-2">
          <div class="flex items-center justify-between text-xs">
            <span class="text-violet-400 font-medium">Running migrations…</span>
            <span class="text-slate-500">{{ migrationCurrent }}/{{ migrationTotal }}</span>
          </div>
          <div class="w-full h-2 bg-slate-700 rounded-full overflow-hidden">
            <div class="h-full bg-gradient-to-r from-violet-500 to-indigo-500 rounded-full transition-all duration-300"
                 :style="{ width: migrationPercent() + '%' }"></div>
          </div>
          <p class="text-[11px] text-slate-500">{{ migrationDesc }}</p>
        </div>

        <p v-if="errorMsg" class="text-xs text-red-400 bg-red-500/10 rounded-lg px-3 py-2">{{ errorMsg }}</p>
      </div>

      <!-- Footer -->
      <div class="flex items-center justify-end px-4 sm:px-6 pb-4 sm:pb-6">
        <button @click="connect" :disabled="connecting || migrating"
                class="flex items-center gap-2 px-5 py-2.5 text-sm rounded-lg font-semibold
                       bg-violet-600 hover:bg-violet-500 text-white transition-colors disabled:opacity-50">
          <svg v-if="connecting" class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4l3-3-3-3v4a8 8 0 000 16v-4l-3 3 3 3v-4a8 8 0 01-8-8z"/>
          </svg>
          {{ connecting ? 'Connecting…' : 'Connect' }}
        </button>
      </div>

    </div>
  </div>
</template>
