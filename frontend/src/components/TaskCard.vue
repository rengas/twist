<script setup>
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

const props = defineProps({ task: Object, colKey: String })

const agentOwnedStatuses = ['prompt', 'code']

const isAgentOwned = agentOwnedStatuses.includes(props.task?.status)
const isProcessing = isAgentOwned && props.task?.approved
const needsApproval = !props.task?.approved

function preview() {
  if (props.task.spec) return props.task.spec.slice(0, 80) + '…'
  return props.task.prompt?.slice(0, 80) + '…'
}

function approvalLabel() {
  const s = props.task.status
  if (s === 'prompt') return 'Approve to spec'
  if (s === 'spec')   return 'Approve to code'
  if (s === 'code')   return 'Approve to build'
  if (s === 'review') return 'Approve PR'
  if (s === 'failed') return 'Retry'
  return ''
}

function openPR() {
  if (props.task.pr_url) {
    BrowserOpenURL(props.task.pr_url)
  }
}
</script>

<template>
  <div class="group rounded-lg bg-slate-900/70 border border-slate-700/50 hover:border-slate-500/70
              cursor-pointer p-3 transition-all duration-150 hover:shadow-lg hover:shadow-black/30
              hover:-translate-y-0.5 active:translate-y-0">

    <!-- Title -->
    <p class="text-xs font-semibold text-slate-200 leading-snug mb-1.5 line-clamp-2">{{ task.title }}</p>

    <!-- Preview -->
    <p class="text-[10px] text-slate-500 leading-relaxed line-clamp-2 mb-2">{{ preview() }}</p>

    <!-- Footer -->
    <div class="flex items-center justify-between mt-auto select-none">
      <span class="text-[10px] text-slate-600 font-mono">#{{ task.id }}</span>

      <!-- Processing spinner -->
      <span v-if="isProcessing" class="flex items-center gap-1 text-[10px] text-emerald-400">
        <svg class="w-2.5 h-2.5 animate-spin" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4l3-3-3-3v4a8 8 0 000 16v-4l-3 3 3 3v-4a8 8 0 01-8-8z"/>
        </svg>
        Running…
      </span>

      <!-- Needs approval badge -->
      <span v-else-if="needsApproval && isAgentOwned"
            class="text-[10px] px-1.5 py-0.5 rounded bg-amber-500/20 text-amber-400 font-medium">
        {{ approvalLabel() }}
      </span>

      <!-- PR link for review/done -->
      <span v-else-if="task.pr_url && (task.status === 'review' || task.status === 'done')"
            class="text-[10px] text-blue-400 hover:text-blue-300 underline truncate max-w-[90px] cursor-pointer"
            @click.stop="openPR">
        {{ task.pr_url.replace(/.*\/pull\//, 'PR #') }}
      </span>

      <!-- Branch chip fallback for review/done -->
      <span v-else-if="task.branch && (task.status === 'review' || task.status === 'done')"
            class="text-[10px] text-sky-400 font-mono truncate max-w-[90px]" :title="task.branch">
        {{ task.branch.replace('feature/', '') }}
      </span>
    </div>
  </div>
</template>
