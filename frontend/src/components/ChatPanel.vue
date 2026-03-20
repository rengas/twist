<script setup>
import { ref, watch, nextTick, computed } from 'vue'
import { marked } from 'marked'

const props = defineProps({
  taskId: Number,
  taskTitle: String,
  messages: Array,
  streaming: Boolean,
  streamBuffer: String,
  error: String
})

const emit = defineEmits(['send', 'close'])

const input = ref('')
const messagesContainer = ref(null)

const displayMessages = computed(() => {
  const msgs = [...(props.messages || [])]
  if (props.streaming && props.streamBuffer) {
    msgs.push({ id: -1, role: 'assistant', content: props.streamBuffer, created_at: '' })
  }
  return msgs
})

function renderMarkdown(content) {
  if (!content) return ''
  return marked.parse(content)
}

function send() {
  const text = input.value.trim()
  if (!text || props.streaming) return
  emit('send', text)
  input.value = ''
}

function onKeydown(e) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    send()
  }
}

function scrollToBottom() {
  nextTick(() => {
    if (messagesContainer.value) {
      messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
    }
  })
}

watch(() => displayMessages.value.length, scrollToBottom)
watch(() => props.streamBuffer, scrollToBottom)
</script>

<template>
  <div class="flex flex-col h-full bg-slate-900 border-l border-slate-700/50">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-slate-700/50 flex-shrink-0">
      <div class="min-w-0 flex-1">
        <p class="text-xs text-slate-500 mb-0.5">Chat</p>
        <p class="text-sm font-semibold text-slate-200 truncate">{{ taskTitle }}</p>
      </div>
      <button @click="emit('close')"
              class="text-slate-500 hover:text-slate-300 transition-colors flex-shrink-0 ml-2">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
        </svg>
      </button>
    </div>

    <!-- Messages -->
    <div ref="messagesContainer" class="flex-1 overflow-y-auto px-3 py-3 space-y-3">
      <div v-if="displayMessages.length === 0" class="flex items-center justify-center h-full">
        <p class="text-xs text-slate-600">No messages yet. Ask Claude about this task.</p>
      </div>

      <div v-for="msg in displayMessages" :key="msg.id"
           :class="msg.role === 'user' ? 'flex justify-end' : 'flex justify-start'">
        <div :class="[
          'max-w-[85%] rounded-lg px-3 py-2 text-sm',
          msg.role === 'user'
            ? 'bg-sky-600/30 text-sky-100'
            : 'bg-slate-800 text-slate-300 border border-slate-700/50'
        ]">
          <div v-if="msg.role === 'assistant'" class="prose prose-sm prose-invert max-w-none" v-html="renderMarkdown(msg.content)"></div>
          <p v-else class="whitespace-pre-wrap">{{ msg.content }}</p>
        </div>
      </div>

      <!-- Streaming indicator -->
      <div v-if="streaming && !streamBuffer" class="flex justify-start">
        <div class="bg-slate-800 border border-slate-700/50 rounded-lg px-3 py-2">
          <div class="flex items-center gap-1.5">
            <span class="w-1.5 h-1.5 bg-slate-400 rounded-full animate-pulse"></span>
            <span class="w-1.5 h-1.5 bg-slate-400 rounded-full animate-pulse" style="animation-delay: 0.2s"></span>
            <span class="w-1.5 h-1.5 bg-slate-400 rounded-full animate-pulse" style="animation-delay: 0.4s"></span>
          </div>
        </div>
      </div>

      <!-- Error -->
      <div v-if="error" class="flex justify-start">
        <div class="bg-red-900/30 border border-red-700/50 rounded-lg px-3 py-2 text-xs text-red-400">
          {{ error }}
        </div>
      </div>
    </div>

    <!-- Input -->
    <div class="flex-shrink-0 border-t border-slate-700/50 px-3 py-3">
      <div class="flex gap-2">
        <textarea v-model="input"
                  @keydown="onKeydown"
                  :disabled="streaming"
                  placeholder="Ask Claude about this task..."
                  rows="2"
                  class="flex-1 resize-none rounded-lg bg-slate-800 border border-slate-700/50 px-3 py-2
                         text-sm text-slate-200 placeholder-slate-600
                         focus:outline-none focus:border-slate-500 disabled:opacity-50"
        ></textarea>
        <button @click="send"
                :disabled="streaming || !input.trim()"
                class="self-end px-3 py-2 rounded-lg bg-violet-600 hover:bg-violet-500
                       text-white text-xs font-medium transition-colors
                       disabled:opacity-50 disabled:cursor-not-allowed flex-shrink-0">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M12 19V5m0 0l-7 7m7-7l7 7" transform="rotate(90 12 12)"/>
          </svg>
        </button>
      </div>
    </div>
  </div>
</template>
