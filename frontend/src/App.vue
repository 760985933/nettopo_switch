<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  createDiscreteApi,
  lightTheme,
  dateDeDE,
  dateEnUS,
  dateEsAR,
  dateFrFR,
  dateJaJP,
  dateKoKR,
  dateZhCN,
  deDE,
  enUS,
  esAR,
  frFR,
  jaJP,
  koKR,
  zhCN,
} from 'naive-ui'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { BrowserOpenURL, ClipboardSetText, EventsOn, Quit, WindowMinimise } from '../wailsjs/runtime/runtime'
import { CheckForUpdates, GetAppVersion } from '../wailsjs/go/main/App'
import SettingsDrawer from './components/SettingsDrawer.vue'
import { useAppStore } from './stores/app'
import { useUiStore } from './stores/ui'

const route = useRoute()
const store = useAppStore()
const ui = useUiStore()
const { t, locale } = useI18n()
const appVersion = ref('v0.0.4')
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

watch(
  () => ui.locale,
  (value) => {
    locale.value = value
  },
  { immediate: true },
)

const naiveLocale = computed(() => {
  switch (ui.locale) {
    case 'zh-CN':
      return zhCN
    case 'ja-JP':
      return jaJP
    case 'ko-KR':
      return koKR
    case 'fr-FR':
      return frFR
    case 'de-DE':
      return deDE
    case 'es-AR':
      return esAR
    default:
      return enUS
  }
})

const naiveDateLocale = computed(() => {
  switch (ui.locale) {
    case 'zh-CN':
      return dateZhCN
    case 'ja-JP':
      return dateJaJP
    case 'ko-KR':
      return dateKoKR
    case 'fr-FR':
      return dateFrFR
    case 'de-DE':
      return dateDeDE
    case 'es-AR':
      return dateEsAR
    default:
      return dateEnUS
  }
})

const { message, dialog } = createDiscreteApi(['message', 'dialog'], {
  configProviderProps: {
    theme: lightTheme,
  },
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

const navItems = computed(() => [
  { label: t('app.nav.overview'), to: '/overview' },
  { label: t('app.nav.sessions'), to: '/sessions' },
  { label: t('app.nav.logs'), to: '/logs' },
  { label: t('app.nav.contact'), to: '/contact' },
])

const statusLabel = computed(() => {
  switch (store.status.status) {
    case 'running':
      return t('app.status.running')
    case 'starting':
      return t('app.status.starting')
    case 'error':
      return t('app.status.error')
    default:
      return t('app.status.stopped')
  }
})

async function handleSaveSettings(config: typeof store.config) {
  try {
    await store.saveConfig(config)
    ui.showSettings = false
    message.success(t('app.toast.settingsSaved'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  }
}

async function handleExport() {
  try {
    const content = await store.exportConfig()
    await ClipboardSetText(content)
    message.success(t('app.toast.configJsonCopied'))
  } catch (error) {
    dialog.warning({
      title: t('app.dialog.exportConfig.title'),
      content: error instanceof Error ? error.message : String(error),
      positiveText: t('app.dialog.exportConfig.ok'),
    })
  }
}

async function handleCodexCopy() {
  try {
    const content = await store.generateCodexConfigToml()
    await ClipboardSetText(content)
    message.success(t('app.toast.codexTomlCopied'))
  } catch (error) {
    dialog.warning({
      title: t('app.dialog.codexCopy.title'),
      content: error instanceof Error ? error.message : String(error),
      positiveText: t('app.dialog.codexCopy.ok'),
    })
  }
}

async function handleCodexWrite() {
  try {
    const path = await store.writeCodexConfigToml()
    const hintPath = await store.getCodexConfigPath()
    message.success(t('app.toast.codexTomlWritten', { path: path || hintPath }))
  } catch (error) {
    dialog.warning({
      title: t('app.dialog.codexWrite.title'),
      content: error instanceof Error ? error.message : String(error),
      positiveText: t('app.dialog.codexWrite.ok'),
    })
  }
}

async function handleCodexWriteProfiles() {
  try {
    const path = await store.writeCodexConfigTomlProfiles()
    const hintPath = await store.getCodexConfigPath()
    message.success(t('app.toast.codexTomlWritten', { path: path || hintPath }))
  } catch (error) {
    dialog.warning({
      title: t('app.dialog.codexWrite.title'),
      content: error instanceof Error ? error.message : String(error),
      positiveText: t('app.dialog.codexWrite.ok'),
    })
  }
}

function handleMinimise() {
  WindowMinimise()
}

function handleClose() {
  Quit()
}

onMounted(async () => {
  try {
    appVersion.value = await GetAppVersion()
  } catch {
  }
  await checkUpdates(false, true)

  // Listen for tray help menu click
  EventsOn('tray:help', () => {
    ui.showHelp = true
  })
})
</script>

<template>
  <n-config-provider :theme="lightTheme" :locale="naiveLocale" :date-locale="naiveDateLocale">
    <n-dialog-provider>
      <n-message-provider placement="bottom-right">
        <div class="shell">
          <header class="topbar">
            <div class="brand">
              <div class="brand-mark">CX</div>
              <div>
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

            <div class="topbar-actions">
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
              <n-button quaternary circle size="small" @click="ui.showHelp = true">
                <template #icon>
                  <span class="help-icon">?</span>
                </template>
              </n-button>
            </div>
          </header>

          <main class="content">
            <RouterView />
          </main>

          <SettingsDrawer
            v-model:model-value="ui.showSettings"
            :config="store.config"
            @save="handleSaveSettings"
            @export="handleExport"
            @codex-copy="handleCodexCopy"
            @codex-write="handleCodexWrite"
            @codex-write-profiles="handleCodexWriteProfiles"
          />

          <!-- Help modal -->
          <n-modal v-model:show="ui.showHelp" preset="card" :title="'💡 ' + t('app.help.title')" style="max-width: 600px" :bordered="false" closable>
            <div class="help-content">
              <div class="help-section">
                <h4>{{ t('app.help.usage.title') }}</h4>
                <ol class="help-steps">
                  <li>{{ t('app.help.usage.step1') }}</li>
                  <li>{{ t('app.help.usage.step2') }}</li>
                  <li>{{ t('app.help.usage.step3') }}</li>
                  <li>{{ t('app.help.usage.step4') }}</li>
                  <li>{{ t('app.help.usage.step5') }}</li>
                </ol>
              </div>
              <div class="help-section">
                <h4>{{ t('app.help.backup.title') }}</h4>
                <p>{{ t('app.help.backup.desc') }}</p>
                <ol class="help-steps">
                  <li>{{ t('app.help.backup.step1') }}</li>
                  <li>{{ t('app.help.backup.step2') }}</li>
                </ol>
                <p class="help-note">{{ t('app.help.backup.note') }}</p>
              </div>
            </div>
          </n-modal>
        </div>
      </n-message-provider>
    </n-dialog-provider>
  </n-config-provider>
</template>

<style scoped>
.shell {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  height: 100vh;
  padding: 20px;
  gap: 18px;
  overflow: hidden;
  scrollbar-gutter: stable;
}

.topbar {
  position: relative;
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 18px;
  padding: 16px 92px 16px 18px;
  border-radius: 24px;
  background: var(--bg-elevated);
  border: 1px solid var(--border);
  backdrop-filter: blur(18px);
  box-shadow: var(--shadow);
}

.brand {
  display: flex;
  align-items: center;
  gap: 14px;
}

.brand-mark {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  border-radius: 14px;
  background: linear-gradient(135deg, rgba(22, 119, 255, 0.16), rgba(19, 194, 194, 0.14));
  color: var(--accent);
  font-weight: 700;
}

.brand p {
  margin: 0 0 4px;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--muted);
}

.brand strong {
  font-size: 16px;
  color: var(--text);
}

.brand-title {
  display: flex;
  align-items: baseline;
  gap: 10px;
}

.app-version {
  font-size: 12px;
  color: var(--muted);
  letter-spacing: 0.04em;
}

.nav {
  display: flex;
  justify-content: center;
  gap: 8px;
}

.nav-link {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 10px 14px;
  border-radius: 999px;
  color: var(--muted);
  text-decoration: none;
  transition: background 160ms ease, color 160ms ease;
}

.nav-link:hover,
.nav-link.active {
  background: rgba(22, 119, 255, 0.08);
  color: var(--text);
}

.topbar-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.window-actions {
  position: absolute;
  top: 12px;
  right: 12px;
  z-index: 2;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.status-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.64);
  border: 1px solid var(--border);
  color: var(--text);
  font-size: 12px;
}

.locale-select {
  width: 132px;
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

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: rgba(11, 18, 32, 0.34);
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

.content {
  min-height: 0;
  min-width: 0;
  overflow-y: auto;
}

@media (max-width: 1024px) {
  .topbar {
    grid-template-columns: 1fr;
  }

  .nav {
    justify-content: flex-start;
    flex-wrap: wrap;
  }

  .topbar-actions {
    justify-content: space-between;
  }
}

@media (max-width: 720px) {
  .shell {
    padding: 12px;
    gap: 12px;
  }

  .topbar {
    padding: 12px 76px 12px 12px;
    gap: 12px;
    border-radius: 18px;
  }

  .status-chip {
    padding: 8px 10px;
  }

  .window-actions {
    top: 10px;
    right: 10px;
  }

  .locale-select {
    width: 118px;
  }
}
</style>

<style>
.help-content {
  display: grid;
  gap: 20px;
}

.help-section h4 {
  margin: 0 0 10px;
  font-size: 15px;
  color: var(--text);
}

.help-steps {
  margin: 0;
  padding-left: 20px;
  line-height: 2;
  font-size: 13px;
  color: var(--text);
}

.help-note {
  margin: 8px 0 0;
  font-size: 12px;
  color: var(--accent);
  opacity: 0.85;
}
</style>
