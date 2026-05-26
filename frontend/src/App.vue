<script setup lang="ts">
import { computed, watch } from 'vue'
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
import { ClipboardSetText, Quit, WindowMinimise } from '../wailsjs/runtime/runtime'
import SettingsDrawer from './components/SettingsDrawer.vue'
import { useAppStore } from './stores/app'
import { useUiStore } from './stores/ui'

const route = useRoute()
const store = useAppStore()
const ui = useUiStore()
const { t, locale } = useI18n()
const appVersion = 'v0.0.3'
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

const navItems = computed(() => [
  { label: t('app.nav.overview'), to: '/overview' },
  { label: t('app.nav.logs'), to: '/logs' },
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

function handleMinimise() {
  WindowMinimise()
}

function handleClose() {
  Quit()
}
</script>

<template>
  <n-config-provider :theme="lightTheme" :locale="naiveLocale" :date-locale="naiveDateLocale">
    <n-dialog-provider>
      <n-message-provider placement="bottom-right">
        <div class="shell">
          <header class="topbar">
            <div class="brand">
              <div class="brand-mark">NT</div>
              <div>
                <p>nettopo.com</p>
                <div class="brand-title">
                  <strong>Nettopo switch</strong>
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
              <n-select
                class="locale-select"
                size="small"
                :value="ui.locale"
                :options="localeOptions"
                @update:value="(value: string) => ui.setLocale(value)"
              />
              <n-button secondary @click="ui.showSettings = true">{{ t('app.actions.preferences') }}</n-button>
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
          />
        </div>
      </n-message-provider>
    </n-dialog-provider>
  </n-config-provider>
</template>

<style scoped>
.shell {
  display: grid;
  min-height: 100vh;
  padding: 20px;
  gap: 18px;
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
  transition: all 160ms ease;
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
