<script setup>
import { computed, ref, watch } from 'vue'
import { marked } from 'marked'
import { BrowserOpenURL, ClipboardSetText } from '../../wailsjs/runtime/runtime'
import { ApproveTask, DeleteTask, UpdateTask, ArchiveTask, RestoreTask } from '../../wailsjs/go/pkg/App'

const props = defineProps({ task: Object })
const emit = defineEmits(['close', 'open-chat'])

const loading = ref(false)
const error = ref('')
const isSpecExpanded = ref(false)

// Edit mode state
const editing = ref(false)
const editTitle = ref('')
const editPrompt = ref('')
const editSpec = ref('')

watch(() => props.task?.id, () => {
  isSpecExpanded.value = false
  editing.value = false
})
const copyTaskFeedback = ref(false)
const copyPRFeedback = ref(false)
const copyBranchFeedback = ref(false)

const renderedSpec = computed(() => {
  if (!props.task.spec) return ''
  return marked.parse(props.task.spec)
})

const isArchived = computed(() => props.task.status === 'archived')

const canApprove = computed(() => {
  const s = props.task.status
  return ['prompt', 'spec', 'code', 'review', 'failed'].includes(s) && !props.task.approved
})

const canEdit = computed(() => {
  const s = props.task.status
  return (s === 'prompt' || s === 'spec') && !props.task.approved
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
  archived: 'bg-slate-500/20 text-slate-300',
}[props.task.status] || 'bg-slate-500/30 text-slate-300'))

function startEditing() {
  editTitle.value = props.task.title
  editPrompt.value = props.task.prompt
  editSpec.value = props.task.spec || ''
  editing.value = true
}

function cancelEditing() {
  editing.value = false
}

async function saveEdit() {
  loading.value = true
  error.value = ''
  try {
    await UpdateTask(props.task.id, editTitle.value, editPrompt.value, editSpec.value)
    editing.value = false
  } catch (e) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
}

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

async function archiveTask() {
  loading.value = true
  error.value = ''
  try {
    await ArchiveTask(props.task.id)
    emit('close')
  } catch (e) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
}

async function restoreTask() {
  loading.value = true
  error.value = ''
  try {
    await RestoreTask(props.task.id)
    emit('close')
  } catch (e) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
}

function openPR() {
  if (props.task.pr_url) {
    BrowserOpenURL(props.task.pr_url)
  }
}

function copyTaskContent() {
  const parts = [`# ${props.task.title}`]
  if (props.task.prompt) parts.push(`\n## Prompt\n${props.task.prompt}`)
  if (props.task.spec) parts.push(`\n## Spec\n${props.task.spec}`)
  if (props.task.pr_url) parts.push(`\nPR: ${props.task.pr_url}`)
  if (props.task.branch) parts.push(`Branch: ${props.task.branch}`)
  parts.push(`Status: ${props.task.status}`)
  ClipboardSetText(parts.join('\n'))
  copyTaskFeedback.value = true
  setTimeout(() => { copyTaskFeedback.value = false }, 1500)
}

function copyField(value, feedbackRef) {
  ClipboardSetText(value)
  feedbackRef.value = true
  setTimeout(() => { feedbackRef.value = false }, 1500)
}
</script>

<template>
  <!-- Backdrop -->
  <div class="fixed inset-0 z-50 flex items-end sm:items-center justify-center p-0 sm:p-6 bg-black/60 backdrop-blur-sm"
       @click.self="emit('close')">
    <!-- Modal -->
    <div class="relative w-full sm:max-w-2xl max-h-[95vh] sm:max-h-[85vh] flex flex-col rounded-t-2xl sm:rounded-2xl
                bg-slate-800 border border-slate-700/60 shadow-2xl shadow-black/50">

      <!-- Header -->
      <div class="flex items-start gap-3 px-4 sm:px-6 pt-4 sm:pt-5 pb-3 sm:pb-4 border-b border-slate-700/50">
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2 mb-1 select-none">
            <span class="text-[10px] font-mono text-slate-500">#{{ task.id }}</span>
            <span class="text-[10px] px-1.5 py-0.5 rounded-full font-semibold uppercase tracking-wide" :class="statusColor">
              {{ task.status }}
            </span>
            <span v-if="task.approved" class="text-[10px] px-1.5 py-0.5 rounded-full bg-emerald-500/20 text-emerald-400 font-semibold">
              Approved
            </span>
          </div>
          <!-- Title: editable or read-only -->
          <input v-if="editing" v-model="editTitle" type="text"
                 class="w-full text-base font-semibold text-slate-100 leading-snug bg-slate-900/50 border border-slate-600 rounded-lg px-3 py-1.5 focus:outline-none focus:border-violet-500" />
          <h2 v-else class="text-base font-semibold text-slate-100 leading-snug line-clamp-3 sm:line-clamp-none">{{ task.title }}</h2>
          <!-- Branch with copy icon -->
          <div v-if="task.branch" class="flex items-center gap-1.5 mt-0.5 min-w-0">
            <p class="text-xs text-sky-400 font-mono truncate">{{ task.branch }}</p>
            <button @click="copyField(task.branch, copyBranchFeedback)"
                    class="p-1 -m-0.5 text-slate-500 hover:text-slate-300 transition-colors select-none flex-shrink-0"
                    :title="copyBranchFeedback ? 'Copied!' : 'Copy branch'">
              <svg v-if="!copyBranchFeedback" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2" stroke-width="2"/>
                <path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1" stroke-width="2"/>
              </svg>
              <svg v-else class="w-3.5 h-3.5 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
              </svg>
            </button>
          </div>
          <!-- PR URL with copy icon -->
          <div v-if="task.pr_url" class="flex items-center gap-1.5 mt-0.5 min-w-0">
            <span class="text-xs text-blue-400 hover:text-blue-300 underline cursor-pointer truncate"
                  @click="openPR">
              {{ task.pr_url }}
              <span class="text-[10px] no-underline"> ↗</span>
            </span>
            <button @click="copyField(task.pr_url, copyPRFeedback)"
                    class="p-1 -m-0.5 text-slate-500 hover:text-slate-300 transition-colors select-none flex-shrink-0"
                    :title="copyPRFeedback ? 'Copied!' : 'Copy PR URL'">
              <svg v-if="!copyPRFeedback" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2" stroke-width="2"/>
                <path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1" stroke-width="2"/>
              </svg>
              <svg v-else class="w-3.5 h-3.5 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
              </svg>
            </button>
          </div>
        </div>
        <div class="flex items-center gap-1.5 flex-shrink-0 mt-0.5 select-none">
          <!-- Edit button -->
          <button v-if="canEdit && !editing" @click="startEditing"
                  class="flex items-center gap-1 px-2 py-1 rounded-lg text-xs text-slate-400 hover:text-slate-200 hover:bg-slate-700 transition-colors"
                  title="Edit task">
            <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                    d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"/>
            </svg>
            Edit
          </button>
          <!-- Copy Task button -->
          <button @click="copyTaskContent"
                  class="flex items-center gap-1 px-2 py-1 rounded-lg text-xs text-slate-400 hover:text-slate-200 hover:bg-slate-700 transition-colors"
                  :title="copyTaskFeedback ? 'Copied!' : 'Copy task content'">
            <svg v-if="!copyTaskFeedback" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <rect x="9" y="9" width="13" height="13" rx="2" ry="2" stroke-width="2"/>
              <path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1" stroke-width="2"/>
            </svg>
            <svg v-else class="w-3.5 h-3.5 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
            </svg>
            {{ copyTaskFeedback ? 'Copied!' : 'Copy' }}
          </button>
          <!-- Close button -->
          <button @click="emit('close')"
                  class="text-slate-500 hover:text-slate-300 transition-colors">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
            </svg>
          </button>
        </div>
      </div>

      <!-- Scrollable content -->
      <div class="overflow-y-auto flex-1 px-4 sm:px-6 py-3 sm:py-4 space-y-4">
        <!-- Archived notice -->
        <div v-if="isArchived"
             class="flex items-center gap-2 px-3 py-2.5 rounded-lg bg-slate-700/50 border border-slate-600/50 text-sm text-slate-300">
          <svg class="w-4 h-4 text-slate-400 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4"/>
          </svg>
          This task is archived. Restore it to start the workflow from the beginning.
        </div>

        <!-- Prompt section -->
        <div>
          <h3 class="text-[10px] uppercase tracking-widest font-semibold text-slate-500 mb-1.5 select-none">Prompt</h3>
          <textarea v-if="editing && task.status === 'prompt'" v-model="editPrompt" rows="5"
                    class="w-full text-sm text-slate-300 bg-slate-900/50 border border-slate-600 rounded-lg px-3 py-2.5 leading-relaxed focus:outline-none focus:border-violet-500 resize-y"></textarea>
          <p v-else class="text-sm text-slate-300 bg-slate-900/50 rounded-lg px-3 py-2.5 leading-relaxed select-text">{{ task.prompt }}</p>
        </div>

        <!-- Spec section (collapsible) -->
        <div v-if="editing && task.status === 'spec'">
          <h3 class="text-[10px] uppercase tracking-widest font-semibold text-slate-500 mb-1.5 select-none">Spec</h3>
          <textarea v-model="editSpec" rows="12"
                    class="w-full text-sm text-slate-300 bg-slate-900/50 border border-slate-600 rounded-lg px-3 py-2.5 leading-relaxed font-mono focus:outline-none focus:border-violet-500 resize-y"></textarea>
        </div>
        <div v-else-if="task.spec" class="spec-section">
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
            <div class="prose prose-sm text-sm bg-slate-900/50 rounded-lg px-3 sm:px-4 py-3 max-w-full overflow-x-auto" v-html="renderedSpec"></div>
          </div>
        </div>
        <div v-else-if="task.status !== 'prompt'" class="text-sm text-slate-600 italic">
          No spec yet — agent will generate it when approved.
        </div>
      </div>

      <!-- Footer actions -->
      <div class="flex flex-wrap items-center justify-between gap-2 px-4 sm:px-6 py-3 sm:py-4 border-t border-slate-700/50 select-none">
        <div class="flex items-center gap-3">
          <button @click="deleteTask" :disabled="loading || editing"
                  class="text-xs text-red-400 hover:text-red-300 transition-colors disabled:opacity-50">
            Delete task
          </button>
          <!-- Archive button for non-archived tasks -->
          <button v-if="!isArchived" @click="archiveTask" :disabled="loading"
                  class="text-xs text-slate-400 hover:text-slate-300 transition-colors disabled:opacity-50">
            Archive
          </button>
        </div>

        <div class="flex flex-wrap items-center gap-2 sm:gap-3">
          <p v-if="error" class="text-xs text-red-400 w-full sm:w-auto">{{ error }}</p>

          <!-- Save / Cancel buttons in edit mode -->
          <template v-if="editing">
            <button @click="cancelEditing"
                    class="px-4 py-2 text-xs rounded-lg bg-slate-700 hover:bg-slate-600 text-slate-300 transition-colors">
              Cancel
            </button>
            <button @click="saveEdit" :disabled="loading"
                    class="flex items-center gap-2 px-4 py-2 text-xs rounded-lg font-semibold
                           bg-violet-600 hover:bg-violet-500 text-white transition-colors disabled:opacity-50">
              <svg v-if="loading" class="w-3.5 h-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4l3-3-3-3v4a8 8 0 000 16v-4l-3 3 3 3v-4a8 8 0 01-8-8z"/>
              </svg>
              Save
            </button>
          </template>

          <!-- Normal action buttons (hidden during editing) -->
          <template v-else>
            <button @click="emit('open-chat', task.id)"
                    class="flex items-center gap-1.5 px-4 py-2 text-xs rounded-lg bg-slate-700 hover:bg-slate-600 text-slate-300 transition-colors">
              <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                      d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z"/>
              </svg>
              Chat
            </button>

            <button @click="emit('close')"
                    class="px-4 py-2 text-xs rounded-lg bg-slate-700 hover:bg-slate-600 text-slate-300 transition-colors">
              Close
            </button>

            <!-- Restore button for archived tasks -->
            <button v-if="isArchived" @click="restoreTask" :disabled="loading"
                    class="flex items-center gap-2 px-4 py-2 text-xs rounded-lg font-semibold
                           bg-emerald-600 hover:bg-emerald-500 text-white transition-colors disabled:opacity-50">
              <svg v-if="loading" class="w-3.5 h-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4l3-3-3-3v4a8 8 0 000 16v-4l-3 3 3 3v-4a8 8 0 01-8-8z"/>
              </svg>
              <svg v-else class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                      d="M3 10h10a5 5 0 010 10H9m4-10l-4-4m4 4l-4 4"/>
              </svg>
              Restore to Prompt
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

            <span v-else-if="task.approved && task.status !== 'done' && !isArchived"
                  class="text-xs text-emerald-400">
              Agent is working…
            </span>
            <span v-else-if="task.status === 'done'"
                  class="text-xs text-emerald-400">
              Done
            </span>
          </template>
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
