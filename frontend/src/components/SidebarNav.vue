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

const iconMap: Record<string, string> = {
  '/overview':
    '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="6" height="6"/><rect x="12" y="2" width="6" height="6"/><rect x="2" y="12" width="6" height="6"/><rect x="12" y="12" width="6" height="6"/></svg>',
  '/proxy':
    '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polyline points="15,1 19,5 15,9"/><path d="M1,9V7a4,4,0,0,1,4-4H19"/><polyline points="5,19 1,15 5,11"/><path d="M19,11v2a4,4,0,0,1-4,4H1"/></svg>',
  '/models':
    '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polygon points="10,2 18,6 10,10 2,6"/><polyline points="2,14 10,18 18,14"/><polyline points="2,10 10,14 18,10"/></svg>',
  '/sessions':
    '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M18,13a2,2,0,0,1-2,2H5L2,18V4a2,2,0,0,1,2-2H16a2,2,0,0,1,2,2Z"/></svg>',
  '/monitoring':
    '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><line x1="15" y1="17" x2="15" y2="8"/><line x1="10" y1="17" x2="10" y2="3"/><line x1="5" y1="17" x2="5" y2="11"/></svg>',
  '/logs':
    '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12,1H5A2,2,0,0,0,3,3V17a2,2,0,0,0,2,2H15a2,2,0,0,0,2-2V6Z"/><polyline points="12,1 12,6 17,6"/><line x1="13.5" y1="11" x2="6.5" y2="11"/><line x1="13.5" y1="14.5" x2="6.5" y2="14.5"/></svg>',
  '/contact':
    '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M17.42,3.84a4.58,4.58,0,0,0-6.48,0L10,4.84l-.94-.94a4.58,4.58,0,0,0-6.48,6.48L3.64,11.44,10,18l6.36-6.57.88-.88a4.58,4.58,0,0,0,0-6.48Z"/></svg>',
}

const navItems = computed(() => [
  { label: t('app.nav.overview'), to: '/overview' },
  { label: t('app.nav.models'), to: '/models' },
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

onMounted(async () => {
  try { appVersion.value = await GetAppVersion() } catch {}
  try { debugMode.value = await GetDebugMode() } catch {}
  await checkUpdates(false, true)
  EventsOn('tray:help', () => { ui.showHelp = true })
})
</script>

<template>
  <aside class="sidebar" :class="{ collapsed }">
    <!-- Sidebar edge toggle (always visible) -->
    <div
      class="edge-toggle"
      @click="ui.toggleSidebar()"
      :title="collapsed ? '展开侧栏' : '收起侧栏'"
    >
      <span class="edge-toggle-icon">{{ collapsed ? '▶' : '◁' }}</span>
    </div>

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
        <span class="nav-icon" v-html="iconMap[item.to]" />
        <span v-show="!collapsed" class="nav-label">{{ item.label }}</span>
      </RouterLink>
    </nav>

    <!-- Footer -->
    <div class="sidebar-footer">
      <div class="footer-row">
        <span class="footer-status-dot" :data-status="store.status.status" />
        <span v-show="!collapsed" class="footer-status-label">{{ statusLabel }}</span>
        <div class="footer-spacer" />
        <span
          v-show="!collapsed && updateHasUpdate"
          class="footer-update-badge"
          @click="checkUpdates(true, true)"
        >
          {{ updateLatest }}
        </span>
        <span class="footer-help" @click="emit('show-help')" title="帮助">?</span>
        <span
          class="footer-debug"
          :class="{ active: debugMode }"
          @click="toggleDebugMode"
          :title="debugMode ? 'Debug: ON' : 'Debug: OFF'"
        />
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
/* ==================== Base ==================== */
.sidebar {
  display: flex;
  flex-direction: column;
  height: 100vh;
  padding: 14px 0;
  gap: 2px;
  background: var(--bg-elevated);
  border-right: 1px solid var(--border);
  backdrop-filter: blur(18px);
  transition: width 200ms ease, min-width 200ms ease;
  overflow: hidden;
  position: relative;
  flex-shrink: 0;
  width: 185px;
  min-width: 185px;
}

.sidebar.collapsed {
  width: 52px !important;
  min-width: 52px !important;
}

/* ==================== Edge toggle ==================== */
.edge-toggle {
  position: absolute;
  top: 50%;
  right: -1px;
  transform: translateY(-50%);
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 44px;
  border-radius: 0 6px 6px 0;
  background: var(--bg-elevated);
  border: 1px solid var(--border);
  border-left: none;
  cursor: pointer;
  transition: background 160ms ease, color 160ms ease;
  color: var(--muted);
}
.edge-toggle:hover {
  background: var(--accent);
  border-color: var(--accent);
  color: #fff;
}

.edge-toggle-icon {
  font-size: 10px;
  line-height: 1;
  user-select: none;
}

/* ==================== Brand header ==================== */
.sidebar-header {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 10px 10px;
  margin-bottom: 2px;
  border-bottom: 1px solid var(--border);
}

.sidebar.collapsed .sidebar-header {
  justify-content: center;
  padding-left: 0;
  padding-right: 0;
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

/* ==================== Navigation ==================== */
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
  margin: 0 6px;
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

.sidebar.collapsed .nav-link {
  justify-content: center;
  padding: 7px 0;
  margin: 0 8px;
}

.nav-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  flex-shrink: 0;
  color: inherit;
}

.nav-icon :deep(svg) {
  width: 20px;
  height: 20px;
}

.nav-label {
  overflow: hidden;
  text-overflow: ellipsis;
}

/* ==================== Footer ==================== */
.sidebar-footer {
  display: flex;
  flex-direction: column;
  padding-top: 8px;
  border-top: 1px solid var(--border);
}

.footer-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px 8px;
}

.sidebar.collapsed .footer-row {
  justify-content: center;
  padding: 6px 0 8px;
}

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

.footer-update-badge {
  font-size: 10px;
  color: var(--warning);
  cursor: pointer;
  flex-shrink: 0;
  padding: 1px 5px;
  border-radius: 4px;
  font-weight: 600;
}
.footer-update-badge:hover {
  background: rgba(255, 170, 0, 0.12);
}

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

.locale-select {
  width: 120px;
  margin: 0 auto 6px;
}
</style>
