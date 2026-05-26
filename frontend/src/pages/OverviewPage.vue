<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { ClipboardSetText } from '../../wailsjs/runtime/runtime'
import ConfigPanel from '../components/ConfigPanel.vue'
import ConsolePanel from '../components/ConsolePanel.vue'
import QuickGuideCard from '../components/QuickGuideCard.vue'
import { useProxyEvents } from '../composables/useProxyEvents'
import { useAppStore } from '../stores/app'
import type { AppConfig } from '../types'

const store = useAppStore()
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

async function handleSave(config: AppConfig) {
  await wrapAction(async () => {
    await store.saveConfig(config)
    return store.refreshStatus()
  }, t('overview.toast.configSaved'))
}

async function handleStart(config: AppConfig) {
  await wrapAction(async () => {
    await store.saveConfig(config)
    return store.startProxy()
  }, t('overview.toast.proxyStarted'), {
    timeoutMs: 5000,
    onTimeout: async () => {
      try {
        await store.stopProxy()
      } finally {
        await store.refreshStatus()
      }
    },
  })
}

async function handleStop() {
  await wrapAction(async () => store.stopProxy(), t('overview.toast.proxyStopped'))
}

async function handleRestart() {
  await wrapAction(async () => store.restartProxy(), t('overview.toast.proxyRestarted'))
}

async function handleHealth() {
  const result = await wrapAction(async () => store.runHealthCheck())
  if (result) {
    message[result.ok ? 'success' : 'warning'](result.ok ? t('overview.health.ok') : t('overview.health.bad'))
  }
}

async function copyText(value: string) {
  await ClipboardSetText(value)
  message.success(t('overview.toast.clipboardCopied'))
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
  <div class="overview-page">
    <div class="page-grid">
      <div class="main-column">
        <ConfigPanel
          :config="store.config"
          :status="store.status"
          :loading="busy"
          @save="handleSave"
          @start="handleStart"
          @stop="handleStop"
          @restart="handleRestart"
          @copy="copyText"
        />
      </div>

      <div class="side-column">
        <QuickGuideCard
          :listen-address="store.status.listenAddress"
          :loading="busy"
          @copy="copyText"
          @health="handleHealth"
        />
        <ConsolePanel
          :status="store.status"
          :health="store.healthCheck"
          :loading="busy"
          @health="handleHealth"
          @refresh="wrapAction(async () => { await store.refreshStatus(); await store.refreshLogs() })"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.overview-page {
  display: grid;
  gap: 24px;
}

.page-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.3fr) minmax(320px, 0.9fr);
  gap: 20px;
}

.main-column,
.side-column {
  display: grid;
  gap: 20px;
}

@media (max-width: 1120px) {
  .page-grid {
    grid-template-columns: 1fr;
  }
}
</style>
