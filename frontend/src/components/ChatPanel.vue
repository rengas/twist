<script setup>
import { ref, watch, nextTick, computed } from 'vue'
import { marked } from 'marked'

const props = defineProps({
  taskId: Number,
  taskTitle: String,
  messages: Array,
  timeline: Array,
  streaming: Boolean,
  streamBuffer: String,
  error: String
})

const emit = defineEmits(['send', 'close'])

const input = ref('')
const messagesContainer = ref(null)

const actorBorderColor = {
  user: 'border-l-sky-500',
  agent: 'border-l-violet-500',
  system: 'border-l-amber-500'
}

const actorLabel = {
  user: 'User',
  agent: 'Claude',
  system: 'System'
}

const displayTimeline = computed(() => {
  const entries = [...(props.timeline || [])]
  // Append streaming message as a synthetic timeline entry
  if (props.streaming && props.streamBuffer) {
    entries.push({
      type: 'message',
      message: { id: -1, role: 'assistant', content: props.streamBuffer, created_at: '' },
      timestamp: ''
    })
  }
  return entries
})

function renderMarkdown(content) {
  if (!content) return ''
  return marked.parse(content)
}

function formatTime(ts) {
  if (!ts) return ''
  try {
    const d = new Date(ts)
    return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  } catch {
    return ts
  }
}

function isLongContent(content) {
  return content && content.length > 200
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

watch(() => displayTimeline.value.length, scrollToBottom)
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

    <!-- Timeline -->
    <div ref="messagesContainer" class="flex-1 overflow-y-auto px-3 py-3 space-y-3">
      <div v-if="displayTimeline.length === 0" class="flex items-center justify-center h-full">
        <p class="text-xs text-slate-600">No messages yet. Ask Claude about this task.</p>
      </div>

      <template v-for="entry in displayTimeline" :key="entry.event?.id || entry.message?.id || Math.random()">
        <!-- Workflow Event -->
        <div v-if="entry.type === 'event' && entry.event" class="flex justify-start">
          <div :class="[
            'w-full rounded-lg px-3 py-2 text-sm bg-slate-800/60 border border-slate-700/30 border-l-2',
            actorBorderColor[entry.event.actor] || 'border-l-slate-500'
          ]">
            <div class="flex items-center gap-2 mb-1">
              <!-- Actor icon -->
              <span v-if="entry.event.actor === 'user'" class="text-sky-400 text-xs" title="User">
                <svg class="w-3.5 h-3.5 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"/>
                </svg>
              </span>
              <span v-else-if="entry.event.actor === 'agent'" class="text-violet-400 text-xs" title="Claude">
                <svg class="w-3.5 h-3.5 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
                </svg>
              </span>
              <span v-else class="text-amber-400 text-xs" title="System">
                <svg class="w-3.5 h-3.5 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/>
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
                </svg>
              </span>
              <span class="font-semibold text-slate-300 text-xs">{{ entry.event.summary }}</span>
              <span class="text-xs text-slate-600 ml-auto">{{ formatTime(entry.event.created_at) }}</span>
            </div>
            <!-- Content: collapsible for long content -->
            <div v-if="entry.event.content">
              <details v-if="isLongContent(entry.event.content)" class="mt-1">
                <summary class="text-xs text-slate-500 cursor-pointer hover:text-slate-400">Show details</summary>
                <div class="mt-1 text-xs text-slate-400 prose prose-sm prose-invert max-w-none overflow-auto max-h-60" v-html="renderMarkdown(entry.event.content)"></div>
              </details>
              <p v-else class="text-xs text-slate-400 mt-1 whitespace-pre-wrap">{{ entry.event.content }}</p>
            </div>
          </div>
        </div>

        <!-- Chat Message -->
        <div v-else-if="entry.type === 'message' && entry.message"
             :class="entry.message.role === 'user' ? 'flex justify-end' : 'flex justify-start'">
          <div :class="[
            'max-w-[min(85%,40rem)] rounded-lg px-3 py-2 text-sm',
            entry.message.role === 'user'
              ? 'bg-sky-600/30 text-sky-100'
              : 'bg-slate-800 text-slate-300 border border-slate-700/50'
          ]">
            <div v-if="entry.message.role === 'assistant'" class="prose prose-sm prose-invert max-w-none" v-html="renderMarkdown(entry.message.content)"></div>
            <p v-else class="whitespace-pre-wrap">{{ entry.message.content }}</p>
          </div>
        </div>
      </template>

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
                  rows="4"
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
