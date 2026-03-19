<script setup>
import { ref } from 'vue'
import { AddTask } from '../../wailsjs/go/pkg/App'

const emit = defineEmits(['close', 'created'])

const title = ref('')
const prompt = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  if (!title.value.trim() || !prompt.value.trim()) {
    error.value = 'Title and prompt are required.'
    return
  }
  loading.value = true
  error.value = ''
  try {
    await AddTask(title.value.trim(), prompt.value.trim())
    emit('created')
    emit('close')
  } catch (e) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center p-6 bg-black/60 backdrop-blur-sm"
       @click.self="emit('close')">
    <div class="w-full max-w-lg rounded-2xl bg-slate-800 border border-slate-700/60 shadow-2xl shadow-black/50">
      <!-- Header -->
      <div class="flex items-center justify-between px-6 pt-5 pb-4 border-b border-slate-700/50">
        <h2 class="text-sm font-semibold text-slate-100">New Task</h2>
        <button @click="emit('close')" class="text-slate-500 hover:text-slate-300 transition-colors">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
          </svg>
        </button>
      </div>

      <!-- Form -->
      <div class="px-6 py-5 space-y-4">
        <div>
          <label class="block text-[10px] uppercase tracking-widest font-semibold text-slate-500 mb-1.5">
            Title
          </label>
          <input v-model="title" type="text" placeholder="Short task name"
                 class="w-full bg-slate-900/60 border border-slate-700 rounded-lg px-3 py-2.5
                        text-sm text-slate-200 placeholder-slate-600 focus:outline-none
                        focus:border-violet-500 focus:ring-1 focus:ring-violet-500/50 transition-colors" />
        </div>

        <div>
          <label class="block text-[10px] uppercase tracking-widest font-semibold text-slate-500 mb-1.5">
            Prompt
          </label>
          <textarea v-model="prompt" rows="5"
                    placeholder="Describe what you want the agent to build or implement…"
                    class="w-full bg-slate-900/60 border border-slate-700 rounded-lg px-3 py-2.5
                           text-sm text-slate-200 placeholder-slate-600 focus:outline-none
                           focus:border-violet-500 focus:ring-1 focus:ring-violet-500/50
                           transition-colors resize-none leading-relaxed"></textarea>
        </div>

        <p v-if="error" class="text-xs text-red-400">{{ error }}</p>
      </div>

      <!-- Footer -->
      <div class="flex items-center justify-end gap-3 px-6 pb-5">
        <button @click="emit('close')"
                class="px-4 py-2 text-xs rounded-lg bg-slate-700 hover:bg-slate-600 text-slate-300 transition-colors">
          Cancel
        </button>
        <button @click="submit" :disabled="loading"
                class="flex items-center gap-2 px-4 py-2 text-xs rounded-lg font-semibold
                       bg-violet-600 hover:bg-violet-500 text-white transition-colors disabled:opacity-50">
          <svg v-if="loading" class="w-3.5 h-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4l3-3-3-3v4a8 8 0 000 16v-4l-3 3 3 3v-4a8 8 0 01-8-8z"/>
          </svg>
          Create Task
        </button>
      </div>
    </div>
  </div>
</template>
