<script setup lang="ts">
import { ref, computed } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { ClipboardSetText } from '../../wailsjs/runtime/runtime'
import { useAppStore } from '../stores/app'
import ProxyStatusCard from './ProxyStatusCard.vue'
import ProxySettingsPanel from './ProxySettingsPanel.vue'
import type { SourceID, ProxyStatusPayload, HealthCheckResult } from '../types'

const props = defineProps<{
  source: SourceID
  status: ProxyStatusPayload
  health: HealthCheckResult | null
  loading: boolean
}>()

const emit = defineEmits<{
  copy: [value: string]
  health: []
  stop: []
  refresh: []
}>()

const store = useAppStore()
const message = useMessage()
const { t } = useI18n()

const showProxySettings = ref(false)

const instanceConfig = computed(() => store.instanceConfig(props.source))

const claudeBaseURL = computed(() => {
  const host = instanceConfig.value?.listenHost || '127.0.0.1'
  const port = instanceConfig.value?.listenPort || 17420
  return `http://${host}:${port}`
})

const envVarCode = computed(() => {
  return `export ANTHROPIC_BASE_URL=${claudeBaseURL.value}\nexport ANTHROPIC_API_KEY=<your-api-key-here>`
})

const proxyHealthURL = computed(() => {
  return `${claudeBaseURL.value}/health`
})

async function copyText(value: string) {
  await ClipboardSetText(value)
  message.success(t('overview.toast.clipboardCopied'))
}
</script>

<template>
  <div class="claude-guide">
    <div class="dashboard-grid">
      <!-- Left column: Status + Env Config -->
      <div class="left-col">
        <!-- Proxy Status -->
        <div class="card">
          <div class="card-header">
            <span class="card-title">{{ t('dashboard.proxyStatus') }}</span>
            <n-button text size="small" type="primary" @click="showProxySettings = true">
              {{ t('dashboard.proxySettings') }}
            </n-button>
          </div>
          <ProxyStatusCard
            :source="source"
            :status="status"
            :loading="loading"
            :health="health"
            @health="emit('health')"
            @refresh="emit('refresh')"
          />
        </div>

        <!-- Environment Variable Setup -->
        <div class="card">
          <div class="card-header">
            <span class="card-title">{{ t('overview.claude.envConfig') }}</span>
          </div>
          <p class="card-desc">{{ t('overview.claude.envConfigDesc') }}</p>
          <div class="code-block">
            <pre><code>{{ envVarCode }}</code></pre>
            <n-button size="small" tertiary @click="copyText(envVarCode)">
              {{ t('logs.actions.copy') }}
            </n-button>
          </div>
          <p class="card-desc" style="margin-top: 12px">{{ t('overview.claude.apiKeyNote') }}</p>
        </div>

        <!-- Verification -->
        <div class="card">
          <div class="card-header">
            <span class="card-title">{{ t('overview.claude.verify') }}</span>
          </div>
          <p class="card-desc">{{ t('overview.claude.verifyDesc') }}</p>
          <div class="code-block">
            <pre><code>claude --version</code></pre>
          </div>
          <p class="card-desc" style="margin-top: 12px">{{ t('overview.claude.runHint') }}</p>
          <div class="code-block">
            <pre><code>{{ `curl ${proxyHealthURL}` }}</code></pre>
            <n-button size="small" tertiary @click="copyText(`curl ${proxyHealthURL}`)">
              {{ t('logs.actions.copy') }}
            </n-button>
          </div>
        </div>
      </div>

      <!-- Right column: Quick actions -->
      <div class="right-col">
        <div class="card">
          <div class="card-header">
            <span class="card-title">{{ t('dashboard.quickActions') }}</span>
          </div>
          <div class="actions">
            <n-button tertiary @click="emit('stop')">{{ t('config.actions.stop') }}</n-button>
            <n-button
              tertiary
              type="primary"
              @click="emit('copy', claudeBaseURL)"
            >
              {{ t('guide.actions.copyBaseUrl') }}
            </n-button>
          </div>
        </div>

        <div class="card">
          <div class="card-header">
            <span class="card-title">{{ t('overview.claude.overview') }}</span>
          </div>
          <p class="card-desc">{{ t('overview.claude.overviewDesc') }}</p>
        </div>
      </div>
    </div>

    <!-- Proxy Settings Drawer -->
    <n-drawer v-model:show="showProxySettings" :width="520" placement="right">
      <n-drawer-content :title="t('proxy.title')" closable>
        <ProxySettingsPanel
          :source="source"
          :config="store.config"
          @save="showProxySettings = false"
        />
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<style scoped>
.claude-guide {
  width: 100%;
}

.dashboard-grid {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: 16px;
  align-items: start;
  min-width: 0;
}

@media (max-width: 900px) {
  .dashboard-grid {
    grid-template-columns: 1fr;
  }
}

.left-col {
  display: grid;
  gap: 16px;
  min-width: 0;
}

.right-col {
  display: grid;
  gap: 16px;
  min-width: 0;
}

.card {
  padding: 16px 18px;
  border-radius: 22px;
  border: 1px solid var(--border);
  background: var(--surface);
  box-shadow: 0 10px 30px rgba(14, 30, 68, 0.08);
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.card-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
}

.card-desc {
  margin: 0 0 10px 0;
  font-size: 12px;
  line-height: 1.6;
  color: rgba(11, 18, 32, 0.64);
}

.code-block {
  position: relative;
  padding: 10px 12px;
  border-radius: 12px;
  border: 1px solid var(--border);
  background: rgba(11, 18, 32, 0.03);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
}

.code-block pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  color: rgba(11, 18, 32, 0.86);
}

.code-block button {
  position: absolute;
  top: 6px;
  right: 6px;
}

.actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
</style>
