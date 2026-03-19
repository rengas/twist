<script setup>
import { ref, reactive, watch } from 'vue'
import { GetSettings, SaveSettings, PickDirectory } from '../../wailsjs/go/main/App'

const props = defineProps({
  modelValue: Boolean,
})
const emit = defineEmits(['update:modelValue', 'saved'])

const draft = reactive({ workDir: '' })
const errorMsg = ref('')
const saving = ref(false)

// Load current settings whenever the modal opens.
watch(() => props.modelValue, async (open) => {
  if (open) {
    errorMsg.value = ''
    try {
      const settings = await GetSettings()
      draft.workDir = settings.workDir ?? ''
    } catch (e) {
      errorMsg.value = String(e)
    }
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
    await SaveSettings({ workDir: draft.workDir })
    emit('saved')
    emit('update:modelValue', false)
  } catch (e) {
    errorMsg.value = String(e)
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
         class="fixed inset-0 z-50 flex items-center justify-center p-6 bg-black/60 backdrop-blur-sm"
         @click.self="cancel"
         @keydown="onKeydown"
         tabindex="-1">
      <div class="w-full max-w-lg rounded-2xl bg-slate-800 border border-slate-700/60 shadow-2xl shadow-black/50"
           role="dialog"
           aria-modal="true"
           aria-label="Settings">

        <!-- Header -->
        <div class="flex items-center justify-between px-6 pt-5 pb-4 border-b border-slate-700/50">
          <h2 class="text-sm font-semibold text-slate-100">Settings</h2>
          <button @click="cancel" class="text-slate-500 hover:text-slate-300 transition-colors">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
            </svg>
          </button>
        </div>

        <!-- Body -->
        <div class="px-6 py-5 space-y-6">

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

          <!-- Future sections slot here -->

          <p v-if="errorMsg" class="text-xs text-red-400">{{ errorMsg }}</p>
        </div>

        <!-- Footer -->
        <div class="flex items-center justify-end gap-3 px-6 pb-5">
          <button @click="cancel"
                  class="px-4 py-2 text-xs rounded-lg bg-slate-700 hover:bg-slate-600 text-slate-300 transition-colors">
            Cancel
          </button>
          <button @click="save" :disabled="saving"
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
