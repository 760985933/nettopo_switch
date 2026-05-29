<script setup lang="ts">
import { ref, computed } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { ClipboardSetText } from '../../wailsjs/runtime/runtime'
import { useAppStore } from '../stores/app'
import { useCodexStore } from '../stores/codex'
import ProxySettingsPanel from '../components/ProxySettingsPanel.vue'
import CodexLoginActions from '../components/CodexLoginActions.vue'

const store = useAppStore()
const codexStore = useCodexStore()
const message = useMessage()
const { t } = useI18n()

const activeTab = ref('general')

const proxyAddress = computed(() => {
  const host = store.config.listenHost || '127.0.0.1'
  const port = store.config.listenPort || 17419
  return `http://${host}:${port}/v1`
})

const codexConfigPath = ref('')
const codexConfigContent = ref('')

async function loadCodexConfig() {
  try {
    codexConfigPath.value = await codexStore.getCodexConfigPath()
    codexConfigContent.value = await codexStore.readCodexConfigToml()
  } catch {
    // file may not exist yet
  }
}

async function handleWriteCodexConfig() {
  try {
    const path = await codexStore.writeCodexConfigTomlRaw(codexConfigContent.value)
    message.success(t('app.toast.codexTomlWritten', { path }))
    await loadCodexConfig()
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  }
}

async function handleCopyProxyAddress() {
  await ClipboardSetText(proxyAddress.value)
  message.success(t('overview.toast.clipboardCopied'))
}

async function handleCopyEnvVar() {
  await ClipboardSetText(`export ANTHROPIC_BASE_URL="${proxyAddress.value}"`)
  message.success(t('overview.toast.clipboardCopied'))
}

function handleTabChange(tab: string) {
  if (tab === 'codex') loadCodexConfig()
}
</script>

<template>
  <div class="proxy-page">
    <div class="page-header">
      <div>
        <h2>{{ t('proxy.title') }}</h2>
        <p class="page-desc">{{ t('proxy.description') }}</p>
      </div>
    </div>

    <n-tabs
      v-model:value="activeTab"
      type="line"
      animated
      @update:value="handleTabChange"
    >
      <!-- Tab 1: General Settings -->
      <n-tab-pane name="general" :tab="t('proxy.tab.general')">
        <ProxySettingsPanel :config="store.config" />
      </n-tab-pane>

      <!-- Tab 2: Codex Desktop -->
      <n-tab-pane name="codex" :tab="t('proxy.tab.codexDesktop')">
        <div class="tab-content">
          <div class="card">
            <div class="card-header">
              <span class="card-title">{{ t('proxy.codex.overview') }}</span>
            </div>
            <p class="card-desc">{{ t('proxy.codex.overviewDesc') }}</p>

            <div class="address-hint">
              <div class="hint-label">{{ t('proxy.proxyAddress') }}</div>
              <div class="hint-row">
                <code>{{ proxyAddress }}</code>
                <n-button text size="small" type="primary" @click="handleCopyProxyAddress">
                  {{ t('guide.actions.copyBaseUrl') }}
                </n-button>
              </div>
            </div>
          </div>

          <div class="card">
            <div class="card-header">
              <span class="card-title">{{ t('proxy.codex.quickLogin') }}</span>
            </div>
            <p class="card-desc">{{ t('proxy.codex.quickLoginDesc') }}</p>
            <div class="login-actions">
              <CodexLoginActions
                v-for="p in store.profileList"
                :key="p.id"
                :profile-id="p.id"
              />
            </div>
            <div v-if="store.profileList.length === 0" class="empty-hint">
              {{ t('dashboard.noProfile') }}
            </div>
          </div>

          <div class="card">
            <div class="card-header">
              <span class="card-title">{{ t('proxy.codex.configToml') }}</span>
            </div>
            <p class="card-desc">{{ t('proxy.codex.configTomlDesc') }}</p>

            <n-form label-placement="top" size="small">
              <n-form-item :label="t('settings.codex.filePath')">
                <n-input :value="codexConfigPath" readonly />
              </n-form-item>
              <n-form-item :label="t('settings.codex.content')">
                <n-input
                  v-model:value="codexConfigContent"
                  type="textarea"
                  :autosize="{ minRows: 8, maxRows: 20 }"
                />
              </n-form-item>
            </n-form>

            <div class="action-row">
              <n-button type="primary" size="small" @click="handleWriteCodexConfig">
                {{ t('settings.codexActions.mergeWrite') }}
              </n-button>
              <n-button size="small" @click="loadCodexConfig">
                {{ t('settings.codexActions.readFile') }}
              </n-button>
            </div>
          </div>
        </div>
      </n-tab-pane>

      <!-- Tab 3: Claude Code -->
      <n-tab-pane name="claude" :tab="t('proxy.tab.claudeCode')">
        <div class="tab-content">
          <div class="card">
            <div class="card-header">
              <span class="card-title">{{ t('proxy.claude.overview') }}</span>
            </div>
            <p class="card-desc">{{ t('proxy.claude.overviewDesc') }}</p>

            <div class="address-hint">
              <div class="hint-label">{{ t('proxy.proxyAddress') }}</div>
              <div class="hint-row">
                <code>{{ proxyAddress }}</code>
                <n-button text size="small" type="primary" @click="handleCopyProxyAddress">
                  {{ t('guide.actions.copyBaseUrl') }}
                </n-button>
              </div>
            </div>
          </div>

          <div class="card">
            <div class="card-header">
              <span class="card-title">{{ t('proxy.claude.envConfig') }}</span>
            </div>
            <p class="card-desc">{{ t('proxy.claude.envConfigDesc') }}</p>

            <div class="code-block">
              <code>export ANTHROPIC_BASE_URL="{{ proxyAddress }}"</code>
              <n-button text size="small" type="primary" @click="handleCopyEnvVar">
                {{ t('logs.actions.copy') }}
              </n-button>
            </div>

            <p class="card-desc" style="margin-top: 16px">{{ t('proxy.claude.apiKeyNote') }}</p>
            <div class="code-block">
              <code>export ANTHROPIC_API_KEY="your-api-key"</code>
            </div>
          </div>

          <div class="card">
            <div class="card-header">
              <span class="card-title">{{ t('proxy.claude.verify') }}</span>
            </div>
            <p class="card-desc">{{ t('proxy.claude.verifyDesc') }}</p>
            <div class="code-block">
              <code>claude --version</code>
            </div>
            <p class="card-desc" style="margin-top: 12px">{{ t('proxy.claude.runHint') }}</p>
            <div class="code-block">
              <code>claude</code>
            </div>
          </div>
        </div>
      </n-tab-pane>
    </n-tabs>
  </div>
</template>

<style scoped>
.proxy-page {
  display: grid;
  gap: 16px;
  max-width: 780px;
}

.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.page-header h2 {
  margin: 0 0 4px;
  font-size: 18px;
  color: var(--text);
}

.page-desc {
  margin: 0;
  font-size: 12px;
  color: var(--muted);
}

.tab-content {
  display: grid;
  gap: 16px;
  padding-top: 8px;
}

.card {
  padding: 16px 18px;
  border-radius: 22px;
  border: 1px solid var(--border);
  background: var(--surface);
  box-shadow: 0 10px 30px rgba(14, 30, 68, 0.08);
}

.card-header {
  margin-bottom: 12px;
}

.card-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
}

.card-desc {
  margin: 0 0 14px;
  font-size: 12px;
  color: var(--muted);
  line-height: 1.6;
}

.address-hint {
  padding: 10px 12px;
  border-radius: 16px;
  border: 1px dashed rgba(22, 119, 255, 0.28);
  background: rgba(22, 119, 255, 0.06);
  display: grid;
  gap: 6px;
}

.hint-label {
  font-size: 12px;
  color: rgba(11, 18, 32, 0.72);
  font-weight: 600;
}

.hint-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.address-hint code {
  font-size: 13px;
  font-weight: 600;
  user-select: all;
  color: rgba(11, 18, 32, 0.9);
}

.login-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.empty-hint {
  font-size: 12px;
  color: var(--muted);
}

.action-row {
  display: flex;
  gap: 8px;
  margin-top: 4px;
}

.code-block {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 12px;
  background: rgba(0, 0, 0, 0.04);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  word-break: break-all;
}

.code-block code {
  flex: 1;
  color: rgba(11, 18, 32, 0.9);
}
</style>
