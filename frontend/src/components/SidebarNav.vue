<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { createDiscreteApi, lightTheme } from 'naive-ui'
import { BrowserOpenURL, EventsOn } from '../../wailsjs/runtime/runtime'
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

const collapsed = computed(() => ui.sidebarCollapsed)

const navItems = computed(() => [
  { label: t('app.nav.overview'), short: '◎', to: '/overview' },
  { label: t('app.nav.models'), short: '◇', to: '/models' },
  { label: t('app.nav.proxy'), short: '⚙', to: '/proxy' },
  { label: t('app.nav.sessions'), short: '☰', to: '/sessions' },
  { label: t('app.nav.monitoring'), short: '📊', to: '/monitoring' },
  { label: t('app.nav.logs'), short: '📋', to: '/logs' },
  { label: t('app.nav.contact'), short: '♥', to: '/contact' },
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

onMounted(async () => {
  try { appVersion.value = await GetAppVersion() } catch {}
  try { debugMode.value = await GetDebugMode() } catch {}
  await checkUpdates(false, true)
  EventsOn('tray:help', () => { ui.showHelp = true })
})
</script>

<template>
  <aside class="sidebar" :class="{ collapsed }">
    <!-- Brand header -->
    <div class="sidebar-header">
      <div class="brand-mark">NT</div>
      <div v-show="!collapsed" class="brand-text">
        <p>www.nettopo.com</p>
        <div class="brand-title">
          <strong>codex switch</strong>
          <span class="app-version">{{ appVersion }}</span>
        </div>
      </div>
    </div>

    <!-- Navigation -->
    <nav class="nav">
      <RouterLink
        v-for="item in navItems"
        :key="item.to"
        :to="item.to"
        class="nav-link"
        :class="{ active: route.path === item.to }"
        :title="collapsed ? item.label : undefined"
      >
        <span class="nav-icon">{{ item.short }}</span>
        <span v-show="!collapsed" class="nav-label">{{ item.label }}</span>
      </RouterLink>
    </nav>

    <!-- Footer: all in one compact row -->
    <div class="sidebar-footer">
      <div class="footer-row">
        <span class="footer-status-dot" :data-status="store.status.status" />
        <span v-show="!collapsed" class="footer-status-label">{{ statusLabel }}</span>

        <div class="footer-spacer" />

        <n-button
          v-show="!collapsed && updateHasUpdate"
          class="footer-update-btn"
          tertiary
          size="tiny"
          type="warning"
          @click="checkUpdates(true, true)"
        >
          {{ updateLatest }}
        </n-button>

        <span
          class="footer-help"
          @click="emit('show-help')"
          title="帮助"
        >?</span>

        <span
          class="footer-debug"
          :class="{ active: debugMode }"
          @click="toggleDebugMode"
          :title="debugMode ? 'Debug: ON' : 'Debug: OFF'"
        />

        <span
          class="footer-collapse"
          @click="ui.toggleSidebar()"
          :title="collapsed ? '展开侧栏' : '收起侧栏'"
        >{{ collapsed ? '▶' : '◁' }}</span>
      </div>

      <n-select
        v-show="!collapsed"
        class="locale-select"
        size="tiny"
        :value="ui.locale"
        :options="localeOptions"
        @update:value="(value: string) => ui.setLocale(value)"
      />
    </div>
  </aside>
</template>

<style scoped>
/* ---- Base sidebar ---- */
.sidebar {
  display: flex;
  flex-direction: column;
  width: 220px;
  min-width: 220px;
  height: 100vh;
  padding: 14px 10px;
  gap: 2px;
  background: var(--bg-elevated);
  border-right: 1px solid var(--border);
  backdrop-filter: blur(18px);
  transition: width 200ms ease, min-width 200ms ease;
  overflow: hidden;
}

.sidebar.collapsed {
  width: 52px;
  min-width: 52px;
}

/* ---- Brand header ---- */
.sidebar-header {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-bottom: 10px;
  margin-bottom: 2px;
  border-bottom: 1px solid var(--border);
}

.brand-mark {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  min-width: 32px;
  height: 32px;
  border-radius: 10px;
  background: linear-gradient(135deg, rgba(22, 119, 255, 0.16), rgba(19, 194, 194, 0.14));
  color: var(--accent);
  font-weight: 700;
  font-size: 13px;
  flex-shrink: 0;
}

.brand-text {
  overflow: hidden;
  white-space: nowrap;
}

.brand-text p {
  margin: 0 0 1px;
  font-size: 9px;
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
  font-size: 13px;
  color: var(--text);
}

.app-version {
  font-size: 9px;
  color: var(--muted);
  letter-spacing: 0.04em;
}

/* ---- Navigation ---- */
.nav {
  display: flex;
  flex-direction: column;
  gap: 1px;
  flex: 1;
  overflow-y: auto;
  padding: 6px 0;
}

.nav-link {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 7px 10px;
  border-radius: 8px;
  color: var(--muted);
  font-size: 13px;
  text-decoration: none;
  transition: background 160ms ease, color 160ms ease;
  white-space: nowrap;
}

.nav-link:hover,
.nav-link.active {
  background: rgba(22, 119, 255, 0.08);
  color: var(--text);
}

.nav-icon {
  font-size: 15px;
  width: 20px;
  text-align: center;
  flex-shrink: 0;
}

.nav-label {
  overflow: hidden;
  text-overflow: ellipsis;
}

/* ---- Footer ---- */
.sidebar-footer {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-top: 8px;
  border-top: 1px solid var(--border);
}

.footer-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

/* Status dot */
.footer-status-dot {
  width: 7px;
  min-width: 7px;
  height: 7px;
  border-radius: 50%;
  background: rgba(11, 18, 32, 0.34);
  flex-shrink: 0;
}
.footer-status-dot[data-status='running'] { background: var(--accent-2); }
.footer-status-dot[data-status='starting'] { background: var(--warning); }
.footer-status-dot[data-status='error'] { background: var(--danger); }

.footer-status-label {
  font-size: 11px;
  color: var(--text);
  white-space: nowrap;
}

.footer-spacer {
  flex: 1;
  min-width: 4px;
}

.footer-update-btn {
  flex-shrink: 0;
}

/* Help button */
.footer-help {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  font-size: 12px;
  font-weight: 700;
  color: var(--accent);
  cursor: pointer;
  flex-shrink: 0;
  transition: background 160ms ease;
}
.footer-help:hover {
  background: rgba(22, 119, 255, 0.1);
}

/* Debug dot */
.footer-debug {
  display: inline-block;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: rgba(11, 18, 32, 0.18);
  cursor: pointer;
  flex-shrink: 0;
  transition: background 200ms ease;
}
.footer-debug.active {
  background: var(--accent);
  box-shadow: 0 0 5px var(--accent);
}

/* Collapse toggle */
.footer-collapse {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  font-size: 10px;
  color: var(--muted);
  cursor: pointer;
  flex-shrink: 0;
  border-radius: 4px;
  transition: background 160ms ease, color 160ms ease;
}
.footer-collapse:hover {
  background: rgba(11, 18, 32, 0.06);
  color: var(--text);
}

/* Locale */
.locale-select {
  width: 100%;
}
</style>
