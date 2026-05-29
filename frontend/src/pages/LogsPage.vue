<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { ClipboardSetText } from '../../wailsjs/runtime/runtime'
import { useAppStore } from '../stores/app'

const store = useAppStore()
const message = useMessage()
const { t } = useI18n()
const loading = ref(false)

const logs = computed(() => store.recentLogs.slice().reverse())

async function refresh() {
  loading.value = true
  try {
    await store.refreshLogs(200)
    message.success(t('logs.toast.refreshed'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    loading.value = false
  }
}

async function copyAll() {
  const text = logs.value
    .map((e) => `[${e.timestamp}] ${e.level.toUpperCase()} ${e.source}: ${e.message}`)
    .join('\n')
  await ClipboardSetText(text)
  message.success(t('logs.toast.copiedAll'))
}

async function copyEntry(entry: typeof logs.value[number]) {
  const text = `[${entry.timestamp}] ${entry.level.toUpperCase()} ${entry.source}: ${entry.message}`
  await ClipboardSetText(text)
  message.success(t('logs.toast.copied'))
}

onMounted(async () => {
  if (!store.lastLoadedAt) {
    await store.initialize()
  } else if (store.recentLogs.length === 0) {
    await refresh()
  }
})
</script>

<template>
  <div class="logs-page">
    <div class="page-head">
      <div>
        <h2>{{ t('logs.title') }}</h2>
        <p>{{ t('logs.desc') }}</p>
      </div>
      <n-space>
        <n-button secondary :loading="loading" @click="refresh">{{ t('logs.actions.refresh') }}</n-button>
        <n-button tertiary :disabled="logs.length === 0" @click="copyAll">{{ t('logs.actions.copyAll') }}</n-button>
      </n-space>
    </div>

    <div v-if="logs.length" class="logs-list">
      <div v-for="entry in logs" :key="entry.id" class="log-row" :data-level="entry.level">
        <div class="meta">
          <span class="badge" :data-level="entry.level">{{ entry.level.toUpperCase() }}</span>
          <span>{{ entry.source }}</span>
          <span class="time">{{ entry.timestamp }}</span>
          <span class="copy-btn" @click="copyEntry(entry)">{{ t('logs.actions.copy') }}</span>
        </div>
        <div class="msg">{{ entry.message }}</div>
      </div>
    </div>
    <div v-else class="logs-empty">
      <n-empty :description="t('logs.empty.noLogs')" />
    </div>
  </div>
</template>

<style scoped>
.logs-page {
  display: grid;
  gap: 14px;
}

.page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 18px;
  border-radius: 22px;
  border: 1px solid var(--border);
  background: var(--surface);
  box-shadow: 0 10px 30px rgba(14, 30, 68, 0.08);
}

.page-head h2 {
  margin: 0 0 6px;
  font-size: 18px;
  color: var(--text);
}

.page-head p {
  margin: 0;
  font-size: 12px;
  color: var(--muted);
}

.logs-list {
  display: grid;
  gap: 10px;
  max-height: calc(100vh - 210px);
  overflow: auto;
  padding-right: 4px;
}

.log-row {
  display: grid;
  gap: 8px;
  padding: 12px 14px;
  border-radius: 18px;
  border: 1px solid var(--border);
  background: rgba(255, 255, 255, 0.82);
}

.log-row[data-level='warn'] {
  border-color: rgba(216, 150, 20, 0.22);
}

.log-row[data-level='error'] {
  border-color: rgba(212, 56, 13, 0.22);
}

.meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 12px;
  color: var(--muted);
}

.badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px 8px;
  border-radius: 999px;
  background: rgba(11, 18, 32, 0.06);
  color: rgba(11, 18, 32, 0.72);
  font-weight: 600;
}

.badge[data-level='warn'] {
  background: rgba(216, 150, 20, 0.14);
  color: var(--warning);
}

.badge[data-level='error'] {
  background: rgba(212, 56, 13, 0.14);
  color: var(--danger);
}

.time {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.copy-btn {
  margin-left: auto;
  font-size: 11px;
  color: var(--accent);
  cursor: pointer;
  opacity: 0;
  transition: opacity 160ms ease;
  flex-shrink: 0;
}
.log-row:hover .copy-btn {
  opacity: 1;
}

.msg {
  font-size: 13px;
  line-height: 1.6;
  color: rgba(11, 18, 32, 0.88);
  word-break: break-word;
}

.logs-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 200px;
  border-radius: 22px;
  border: 1px solid var(--border);
  background: var(--surface);
}

@media (max-width: 920px) {
  .page-head {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
