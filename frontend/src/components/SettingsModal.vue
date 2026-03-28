<script setup>
import { ref, reactive, watch } from 'vue'
import { GetSettings, SaveSettings, PickDirectory, ConnectDB } from '../../wailsjs/go/pkg/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

const props = defineProps({
  modelValue: Boolean,
})
const emit = defineEmits(['update:modelValue', 'saved'])

const draft = reactive({ workDir: '', maxWorkers: '3', databaseURL: '' })
const originalDbUrl = ref('')
const errorMsg = ref('')
const saving = ref(false)
const reconnecting = ref(false)

// Migration progress (reused pattern from ConnectionModal).
const migrating = ref(false)
const migrationCurrent = ref(0)
const migrationTotal = ref(0)
const migrationDesc = ref('')

// Load current settings whenever the modal opens.
watch(() => props.modelValue, async (open) => {
  if (open) {
    errorMsg.value = ''
    try {
      const settings = await GetSettings()
      draft.workDir = settings.workDir ?? ''
      draft.maxWorkers = settings.maxWorkers ?? '3'
      draft.databaseURL = settings.databaseURL ?? ''
      originalDbUrl.value = draft.databaseURL
    } catch (e) {
      errorMsg.value = String(e)
    }
    EventsOn('migration:status', (status) => {
      migrating.value = status.running
      migrationCurrent.value = status.current
      migrationTotal.value = status.total
      migrationDesc.value = status.description
    })
  } else {
    EventsOff('migration:status')
    migrating.value = false
  }
})

async function browse() {
  try {
    const chosen = await PickDirectory()
    if (chosen) {
      draft.workDir = chosen
    }
  } catch (e) {
    errorMsg.value = String(e)
  }
}

async function save() {
  saving.value = true
  errorMsg.value = ''
  try {
    // If database URL changed, reconnect first.
    const urlChanged = draft.databaseURL.trim() !== originalDbUrl.value.trim()
    if (urlChanged && draft.databaseURL.trim()) {
      reconnecting.value = true
      await ConnectDB(draft.databaseURL.trim())
      reconnecting.value = false
      originalDbUrl.value = draft.databaseURL.trim()
      // ConnectDB already saves the URL to config and emits db:status + tasks:updated
    }

    // Save remaining settings (workDir, maxWorkers) via the existing path.
    await SaveSettings({ workDir: draft.workDir, maxWorkers: draft.maxWorkers })
    emit('saved')
    emit('update:modelValue', false)
  } catch (e) {
    errorMsg.value = String(e)
    reconnecting.value = false
    migrating.value = false
  } finally {
    saving.value = false
  }
}

function cancel() {
  emit('update:modelValue', false)
}

function onKeydown(e) {
  if (e.key === 'Escape') cancel()
}
</script>

<template>
  <Teleport to="body">
    <div v-if="modelValue"
         class="fixed inset-0 z-50 flex items-center justify-center p-4 sm:p-6 bg-black/60 backdrop-blur-sm"
         @click.self="cancel"
         @keydown="onKeydown"
         tabindex="-1">
      <div class="w-full max-w-[calc(100vw-2rem)] sm:max-w-lg max-h-[92vh] sm:max-h-[85vh] overflow-y-auto rounded-2xl bg-slate-800 border border-slate-700/60 shadow-2xl shadow-black/50"
           role="dialog"
           aria-modal="true"
           aria-label="Settings">

        <!-- Header -->
        <div class="flex items-center justify-between px-4 sm:px-6 pt-5 pb-4 border-b border-slate-700/50">
          <h2 class="text-sm font-semibold text-slate-100">Settings</h2>
          <button @click="cancel" class="text-slate-500 hover:text-slate-300 transition-colors">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
            </svg>
          </button>
        </div>

        <!-- Body -->
        <div class="px-4 sm:px-6 py-5 space-y-6">

          <!-- Database section -->
          <section>
            <h3 class="text-[10px] uppercase tracking-widest font-semibold text-slate-500 mb-3">Database</h3>

            <div class="space-y-1.5">
              <label class="block text-xs font-medium text-slate-400">PostgreSQL URL</label>
              <input v-model="draft.databaseURL"
                     type="text"
                     :disabled="reconnecting || migrating"
                     placeholder="postgres://user:pass@localhost:5432/twist?sslmode=disable"
                     class="w-full bg-slate-900/60 border border-slate-700 rounded-lg px-3 py-2
                            text-sm text-slate-200 placeholder-slate-600 font-mono focus:outline-none
                            focus:border-violet-500 focus:ring-1 focus:ring-violet-500/50 transition-colors
                            disabled:opacity-50" />
              <p class="text-[11px] text-slate-500">
                Changing this will reconnect, run migrations, and reload all tasks.
              </p>
            </div>

            <!-- Migration progress bar (shown during reconnect) -->
            <div v-if="migrating" class="mt-3 space-y-2">
              <div class="flex items-center justify-between text-xs">
                <span class="text-violet-400 font-medium">Running migrations...</span>
                <span class="text-slate-500">{{ migrationCurrent }}/{{ migrationTotal }}</span>
              </div>
              <div class="w-full h-2 bg-slate-700 rounded-full overflow-hidden">
                <div class="h-full bg-gradient-to-r from-violet-500 to-indigo-500 rounded-full transition-all duration-300"
                     :style="{ width: (migrationTotal > 0 ? Math.round((migrationCurrent / migrationTotal) * 100) : 0) + '%' }"></div>
              </div>
              <p class="text-[11px] text-slate-500">{{ migrationDesc }}</p>
            </div>
          </section>

          <!-- Project section -->
          <section>
            <h3 class="text-[10px] uppercase tracking-widest font-semibold text-slate-500 mb-3">Project</h3>

            <div class="space-y-1.5">
              <label class="block text-xs font-medium text-slate-400">Working Directory</label>
              <div class="flex gap-2">
                <input v-model="draft.workDir"
                       type="text"
                       placeholder="/path/to/project"
                       class="flex-1 bg-slate-900/60 border border-slate-700 rounded-lg px-3 py-2
                              text-sm text-slate-200 placeholder-slate-600 focus:outline-none
                              focus:border-violet-500 focus:ring-1 focus:ring-violet-500/50 transition-colors" />
                <button @click="browse"
                        class="px-3 py-2 text-xs rounded-lg bg-slate-700 hover:bg-slate-600 text-slate-300
                               transition-colors whitespace-nowrap">
                  Browse
                </button>
              </div>
              <p class="text-[11px] text-slate-500">
                Directory where twist.db and task branches are created.
              </p>
            </div>
          </section>

          <!-- Agent section -->
          <section>
            <h3 class="text-[10px] uppercase tracking-widest font-semibold text-slate-500 mb-3">Agent</h3>

            <div class="space-y-1.5">
              <label class="block text-xs font-medium text-slate-400">Max Concurrent Tasks</label>
              <input v-model="draft.maxWorkers"
                     type="number"
                     min="1"
                     max="10"
                     class="w-24 bg-slate-900/60 border border-slate-700 rounded-lg px-3 py-2
                            text-sm text-slate-200 focus:outline-none
                            focus:border-violet-500 focus:ring-1 focus:ring-violet-500/50 transition-colors" />
              <p class="text-[11px] text-slate-500">
                Number of tasks the agent can process in parallel (1–10).
              </p>
            </div>
          </section>

          <p v-if="errorMsg" class="text-xs text-red-400">{{ errorMsg }}</p>
        </div>

        <!-- Footer -->
        <div class="flex items-center justify-end gap-3 px-4 sm:px-6 pb-5">
          <button @click="cancel"
                  class="px-4 py-2 text-xs rounded-lg bg-slate-700 hover:bg-slate-600 text-slate-300 transition-colors">
            Cancel
          </button>
          <button @click="save" :disabled="saving || reconnecting || migrating"
                  class="flex items-center gap-2 px-4 py-2 text-xs rounded-lg font-semibold
                         bg-violet-600 hover:bg-violet-500 text-white transition-colors disabled:opacity-50">
            <svg v-if="saving" class="w-3.5 h-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4l3-3-3-3v4a8 8 0 000 16v-4l-3 3 3 3v-4a8 8 0 01-8-8z"/>
            </svg>
            Save
          </button>
        </div>

      </div>
    </div>
  </Teleport>
</template>
