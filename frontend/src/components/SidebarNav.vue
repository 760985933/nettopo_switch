<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { createDiscreteApi, lightTheme } from 'naive-ui'
import { BrowserOpenURL, Quit, WindowMinimise } from '../../wailsjs/runtime/runtime'
import { CheckForUpdates, GetAppVersion, GetDebugMode, SetDebugMode } from '../../wailsjs/go/main/App'
import { useAppStore } from '../stores/app'
import { useUiStore } from '../stores/ui'

const { message, dialog } = createDiscreteApi(['message', 'dialog'], {
  configProviderProps: { theme: lightTheme },
})

const emit = defineEmits<{
  'show-help': []
}>()

const route = useRoute()
const store = useAppStore()
const ui = useUiStore()
const { t } = useI18n()

const appVersion = ref('v0.0.4')
const debugMode = ref(false)
const updateChecking = ref(false)
const updateHasUpdate = ref(false)
const updateLatest = ref('')
const updateURL = ref('')

const localeOptions = [
  { label: '简体中文', value: 'zh-CN' },
  { label: 'English', value: 'en-US' },
  { label: '日本語', value: 'ja-JP' },
  { label: '한국어', value: 'ko-KR' },
  { label: 'Français', value: 'fr-FR' },
  { label: 'Deutsch', value: 'de-DE' },
  { label: 'Español', value: 'es-AR' },
] as const

const navItems = computed(() => [
  { label: t('app.nav.overview'), to: '/overview' },
  { label: t('app.nav.models'), to: '/models' },
  { label: t('app.nav.proxy'), to: '/proxy' },
  { label: t('app.nav.sessions'), to: '/sessions' },
  { label: t('app.nav.monitoring'), to: '/monitoring' },
  { label: t('app.nav.logs'), to: '/logs' },
  { label: t('app.nav.contact'), to: '/contact' },
])

const statusLabel = computed(() => {
  switch (store.status.status) {
    case 'running': return t('app.status.running')
    case 'starting': return t('app.status.starting')
    case 'error': return t('app.status.error')
    default: return t('app.status.stopped')
  }
})

async function checkUpdates(showUpToDateToast: boolean, showDialogOnUpdate: boolean) {
  if (updateChecking.value) return
  updateChecking.value = true
  try {
    const result = await CheckForUpdates()
    updateHasUpdate.value = !!result.hasUpdate
    updateLatest.value = result.latestVersion || ''
    updateURL.value = result.downloadUrl || ''

    if (result.hasUpdate) {
      if (showDialogOnUpdate) {
        dialog.info({
          title: '发现新版本',
          content:
            `当前版本：${result.currentVersion || appVersion.value}\n` +
            `最新版本：${result.latestVersion || ''}\n\n` +
            `${result.notes || '请下载更新。'}`,
          positiveText: updateURL.value ? '下载更新' : undefined,
          negativeText: '稍后',
          onPositiveClick: () => {
            BrowserOpenURL(updateURL.value)
          },
        })
      }
      return
    }

    if (showUpToDateToast) {
      message.success('已是最新版本')
    }
  } catch (error) {
    const text = error instanceof Error ? error.message : String(error)
    if (text.includes('未配置更新地址')) {
      message.warning('未配置更新地址')
    } else {
      message.error(text)
    }
  } finally {
    updateChecking.value = false
  }
}

async function toggleDebugMode() {
  debugMode.value = !debugMode.value
  await SetDebugMode(debugMode.value)
}

function handleMinimise() { WindowMinimise() }

function handleClose() { Quit() }

onMounted(async () => {
  try { appVersion.value = await GetAppVersion() } catch {}
  try { debugMode.value = await GetDebugMode() } catch {}
  await checkUpdates(false, true)
})
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-header">
      <div class="brand">
        <div class="brand-mark">CX</div>
        <div class="brand-text">
          <p>www.nettopo.com</p>
          <div class="brand-title">
            <strong>codex switch</strong>
            <span class="app-version">{{ appVersion }}</span>
          </div>
        </div>
      </div>
      <div class="window-actions">
        <n-button tertiary size="small" @click="handleMinimise">—</n-button>
        <n-button tertiary type="error" size="small" @click="handleClose">×</n-button>
      </div>
    </div>

    <nav class="nav">
      <RouterLink
        v-for="item in navItems"
        :key="item.to"
        :to="item.to"
        class="nav-link"
        :class="{ active: route.path === item.to }"
      >
        {{ item.label }}
      </RouterLink>
    </nav>

    <div class="sidebar-footer">
      <div class="status-chip" :data-status="store.status.status">
        <span class="status-dot" />
        {{ statusLabel }}
      </div>
      <n-button
        secondary
        size="small"
        :loading="updateChecking"
        :type="updateHasUpdate ? 'warning' : undefined"
        @click="checkUpdates(true, true)"
      >
        {{ updateHasUpdate ? `有更新 ${updateLatest}` : '检查更新' }}
      </n-button>
      <n-select
        class="locale-select"
        size="small"
        :value="ui.locale"
        :options="localeOptions"
        @update:value="(value: string) => ui.setLocale(value)"
      />
      <div class="footer-actions">
        <n-button quaternary circle size="small" @click="emit('show-help')">
          <template #icon><span class="help-icon">?</span></template>
        </n-button>
        <span
          class="debug-dot"
          :class="{ active: debugMode }"
          @click="toggleDebugMode"
          :title="debugMode ? 'Debug: ON' : 'Debug: OFF'"
        />
      </div>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  display: flex;
  flex-direction: column;
  width: 220px;
  min-width: 220px;
  height: 100vh;
  padding: 16px 12px;
  gap: 4px;
  background: var(--bg-elevated);
  border-right: 1px solid var(--border);
  backdrop-filter: blur(18px);
}

.sidebar-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding-bottom: 12px;
  margin-bottom: 4px;
  border-bottom: 1px solid var(--border);
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
}

.brand-mark {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 12px;
  background: linear-gradient(135deg, rgba(22, 119, 255, 0.16), rgba(19, 194, 194, 0.14));
  color: var(--accent);
  font-weight: 700;
  font-size: 14px;
  flex-shrink: 0;
}

.brand-text p {
  margin: 0 0 2px;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--muted);
}

.brand-title {
  display: flex;
  align-items: baseline;
  gap: 6px;
}

.brand-title strong {
  font-size: 14px;
  color: var(--text);
}

.app-version {
  font-size: 10px;
  color: var(--muted);
  letter-spacing: 0.04em;
}

.window-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  overflow-y: auto;
  padding: 8px 0;
}

.nav-link {
  display: flex;
  align-items: center;
  padding: 8px 12px;
  border-radius: 10px;
  color: var(--muted);
  font-size: 13px;
  text-decoration: none;
  transition: background 160ms ease, color 160ms ease;
}

.nav-link:hover,
.nav-link.active {
  background: rgba(22, 119, 255, 0.08);
  color: var(--text);
}

.sidebar-footer {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-top: 12px;
  border-top: 1px solid var(--border);
}

.status-chip {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.56);
  border: 1px solid var(--border);
  color: var(--text);
  font-size: 12px;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: rgba(11, 18, 32, 0.34);
  flex-shrink: 0;
}

.status-chip[data-status='running'] .status-dot {
  background: var(--accent-2);
}

.status-chip[data-status='starting'] .status-dot {
  background: var(--warning);
}

.status-chip[data-status='error'] .status-dot {
  background: var(--danger);
}

.locale-select {
  width: 100%;
}

.footer-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.help-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  font-size: 14px;
  font-weight: 700;
  color: var(--accent);
  cursor: pointer;
}

.debug-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: rgba(11, 18, 32, 0.18);
  cursor: pointer;
  transition: background 200ms ease;
  flex-shrink: 0;
}

.debug-dot.active {
  background: var(--accent);
  box-shadow: 0 0 6px var(--accent);
}
</style>
