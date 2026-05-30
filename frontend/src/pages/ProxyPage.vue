<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { ClipboardSetText } from '../../wailsjs/runtime/runtime'
import QuickGuideCard from '../components/QuickGuideCard.vue'
import ClaudeSetupGuide from '../components/ClaudeSetupGuide.vue'
import { useProxyEvents } from '../composables/useProxyEvents'
import { useAppStore } from '../stores/app'
import { useUiStore } from '../stores/ui'
import type { SourceID } from '../types'

const store = useAppStore()
const ui = useUiStore()
const message = useMessage()
const { t } = useI18n()
const busy = ref(false)

async function wrapAction<T>(
  task: () => Promise<T>,
  successMessage?: string,
  options?: { timeoutMs?: number; onTimeout?: () => Promise<void> },
) {
  busy.value = true
  try {
    const timeoutMs = options?.timeoutMs ?? 5000
    const timeoutSeconds = Math.max(1, Math.round(timeoutMs / 1000))
    const timeoutError = new Error(t('app.errors.timeoutStopped', { seconds: timeoutSeconds }))
    timeoutError.name = 'TimeoutError'
    const timeoutPromise = new Promise<never>((_, reject) => {
      window.setTimeout(() => reject(timeoutError), timeoutMs)
    })

    const result = await Promise.race([task(), timeoutPromise])
    await store.refreshLogs()
    if (successMessage) {
      message.success(successMessage)
    }
    return result
  } catch (error) {
    if (error instanceof Error && error.name === 'TimeoutError') {
      if (options?.onTimeout) {
        await options.onTimeout()
      }
      await store.refreshLogs()
      message.error(error.message)
      return null as T
    }

    message.error(error instanceof Error ? error.message : String(error))
    throw error
  } finally {
    busy.value = false
  }
}

async function handleStop(source: SourceID) {
  await wrapAction(async () => store.stopProxyForSource(source), t('overview.toast.proxyStopped'))
}

async function handleHealth(source: SourceID) {
  const result = await wrapAction(async () => store.runHealthCheckForSource(source))
  if (result) {
    message[result.ok ? 'success' : 'warning'](result.ok ? t('overview.health.ok') : t('overview.health.bad'))
  }
}

async function copyText(value: string) {
  await ClipboardSetText(value)
  message.success(t('overview.toast.clipboardCopied'))
}

const activeTab = ref<SourceID>('codex')

const tabs = [
  {
    key: 'codex' as SourceID,
    label: t('overview.tab.codexDesktop'),
    icon: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>',
  },
  {
    key: 'claude' as SourceID,
    label: t('overview.tab.claudeCode'),
    icon: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>',
  },
]

async function handleRefresh(source: SourceID) {
  await wrapAction(async () => {
    await store.refreshStatus(source)
    await store.refreshLogs()
  })
}

useProxyEvents({
  onStatus(payload) {
    store.applyStatus(payload)
  },
  onLog(entry) {
    store.pushLog(entry)
  },
})

onMounted(async () => {
  if (!store.lastLoadedAt) {
    await wrapAction(async () => store.initialize())
  }
})
</script>

<template>
  <div class="proxy-page">
    <div class="tab-bar">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        class="tab-btn"
        :class="{ active: activeTab === tab.key }"
        @click="activeTab = tab.key"
      >
        <span class="tab-icon" v-html="tab.icon" />
        <span class="tab-label">{{ tab.label }}</span>
      </button>
    </div>

    <div v-show="activeTab === 'codex'">
      <QuickGuideCard
        :source="'codex'"
        :listen-address="store.statuses.codex.listenAddress"
        :status="store.statuses.codex"
        :health="store.healthCheckForSource('codex')"
        :loading="busy"
        @copy="copyText"
        @health="handleHealth('codex')"
        @stop="handleStop('codex')"
        @refresh="handleRefresh('codex')"
      />
    </div>

    <div v-show="activeTab === 'claude'">
      <ClaudeSetupGuide
        :source="'claude'"
        :status="store.statuses.claude"
        :health="store.healthCheckForSource('claude')"
        :loading="busy"
        @copy="copyText"
        @health="handleHealth('claude')"
        @stop="handleStop('claude')"
        @refresh="handleRefresh('claude')"
      />
    </div>
  </div>
</template>

<style scoped>
.proxy-page {
  display: grid;
  gap: 16px;
  width: 100%;
}
/* ── Tab bar ── */
.tab-bar {
  display: inline-flex;
  gap: 4px;
  padding: 4px;
  border-radius: 10px;
  background: rgba(11, 18, 32, 0.04);
  width: fit-content;
}

.tab-btn {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 7px 16px;
  border: none;
  border-radius: 7px;
  background: transparent;
  color: rgba(11, 18, 32, 0.48);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.18s ease;
  outline: none;
  font-family: inherit;
  white-space: nowrap;
}

.tab-btn:hover {
  color: rgba(11, 18, 32, 0.72);
  background: rgba(11, 18, 32, 0.04);
}

.tab-btn.active {
  color: rgba(11, 18, 32, 0.92);
  background: #fff;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06), 0 1px 2px rgba(0, 0, 0, 0.04);
  font-weight: 600;
}

.tab-icon {
  display: flex;
  align-items: center;
  opacity: 0.7;
}

.tab-btn.active .tab-icon {
  opacity: 1;
}
</style>
