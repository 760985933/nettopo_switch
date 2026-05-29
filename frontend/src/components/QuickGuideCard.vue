<script setup lang="ts">
import { ref, computed } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '../stores/app'
import { useUiStore } from '../stores/ui'
import { getProviderPreset } from '../utils/providers'
import ProfileList from './ProfileList.vue'
import ModelEditorPanel from './ModelEditorPanel.vue'
import ProxySettingsPanel from './ProxySettingsPanel.vue'
import CodexLoginActions from './CodexLoginActions.vue'
import type { ProxyStatusPayload, HealthCheckResult } from '../types'

const emit = defineEmits<{
  copy: [value: string]
  health: []
  stop: []
  refresh: []
}>()

const props = defineProps<{
  listenAddress: string
  loading: boolean
  status: ProxyStatusPayload
  health: HealthCheckResult | null
}>()

const store = useAppStore()
const ui = useUiStore()
const router = useRouter()
const message = useMessage()
const { t } = useI18n()

const statusLabel = computed(() => {
  switch (props.status.status) {
    case 'running': return t('app.status.running')
    case 'starting': return t('app.status.starting')
    case 'error': return t('app.status.error')
    default: return t('app.status.stopped')
  }
})

const healthSummary = computed(() => {
  if (!props.health) return null
  const failed = props.health.checks.filter((item) => !item.ok)
  if (props.health.ok) return { tone: 'success' as const, text: t('console.health.ok') }
  return { tone: 'warning' as const, text: t('console.health.failed', { count: failed.length }) }
})

const failedChecks = computed(() => {
  if (!props.health) return []
  return props.health.checks.filter((item) => !item.ok)
})

const codexBaseURL = computed(() => {
  if (!props.listenAddress) return ''
  return props.listenAddress.replace(/\/+$/, '') + '/v1'
})

const currentProfile = computed(() => store.currentProfile)

// ── Add profile ──
const showAddDialog = ref(false)
const newProfileName = ref('')
const newProfileProvider = ref('deepseek')
const newProfileApiKey = ref('')

const providerOptions = [
  { label: 'DeepSeek', value: 'deepseek' },
  { label: '阿里通义千问', value: 'alibaba' },
  { label: '小米 MiMo', value: 'xiaomi' },
  { label: '智谱 GLM', value: 'zhipu' },
  { label: '百度千帆', value: 'baidu' },
  { label: '火山引擎豆包', value: 'volcano' },
  { label: '腾讯混元', value: 'tencent' },
  { label: '硅基流动', value: 'silicon' },
  { label: 'Kimi', value: 'kimi' },
  { label: 'MiniMax 海螺', value: 'minimax' },
  { label: 'Google Gemini', value: 'google' },
  { label: 'Anthropic Claude', value: 'anthropic' },
  { label: '自定义', value: 'custom' },
]

async function handleAddProfile() {
  if (!newProfileName.value.trim()) return
  await store.addProfile(newProfileName.value.trim(), newProfileProvider.value, undefined, newProfileApiKey.value || undefined)
  newProfileName.value = ''
  newProfileProvider.value = 'deepseek'
  newProfileApiKey.value = ''
  showAddDialog.value = false
  message.success(t('models.toast.added'))
}

// ── Edit / Delete ──
const editingProfileId = ref<string | null>(null)
const showEditor = ref(false)

// ── Proxy settings drawer ──
const showProxySettings = ref(false)

function handleEdit(id: string) {
  editingProfileId.value = id
  showEditor.value = true
}

function handleDelete(id: string) {
  if (store.profileList.length < 2) {
    message.warning(t('profile.cannotDeleteLast'))
    return
  }
  store.deleteProfile(id)
}

function handleEditorSave() {
  showEditor.value = false
  editingProfileId.value = null
}
</script>

<template>
  <div class="dashboard">
    <div class="dashboard-grid">
      <!-- Left: Profile Card -->
      <div class="card">
        <div class="card-header">
          <span class="card-title">{{ t('dashboard.currentProfile') }}</span>
          <div class="card-header-actions">
            <n-button text size="small" @click="showAddDialog = true">
              {{ t('models.addProfile') }}
            </n-button>
            <n-button text size="small" type="primary" @click="router.push('/models')">
              {{ t('dashboard.manageModels') }}
            </n-button>
          </div>
        </div>

        <ProfileList
          :profiles="store.profileList"
          :current-profile-id="store.config.currentProfileId"
          :loading="store.isBusy"
          :show-delete="false"
          @edit="handleEdit"
          @delete="handleDelete"
          @select="store.setCurrentProfile"
        >
          <template #actions="{ profile }">
            <CodexLoginActions :profile-id="profile.id" />
          </template>
        </ProfileList>
      </div>

      <!-- Right: Status + Actions -->
      <div class="right-col">
        <!-- Proxy Status Card -->
        <div class="card">
          <div class="card-header">
            <span class="card-title">{{ t('dashboard.proxyStatus') }}</span>
            <n-button text size="small" type="primary" @click="showProxySettings = true">
              {{ t('dashboard.proxySettings') }}
            </n-button>
          </div>
          <div class="status-section">
            <div class="s-status">
              <span class="s-dot" :data-status="status.status" />
              <span>{{ statusLabel }}</span>
            </div>

            <div class="s-meta">
              <span class="s-meta-item">
                <span class="s-meta-label">{{ t('console.meta.listenAddress') }}:</span>
                <strong>{{ status.listenAddress || t('console.meta.notRunning') }}</strong>
              </span>
              <span class="s-meta-item">
                <span class="s-meta-label">{{ t('console.meta.requestCount') }}:</span>
                <strong>{{ status.requestCount }}</strong>
              </span>
              <span v-if="status.lastError" class="s-meta-item" data-tone="error">
                <span class="s-meta-label">{{ t('console.meta.lastError') }}:</span>
                <strong>{{ status.lastError }}</strong>
              </span>
            </div>

            <div v-if="healthSummary" class="s-health" :data-tone="healthSummary.tone">
              <span class="h-dot" />
              <span>{{ healthSummary.text }}</span>
            </div>
            <div v-if="failedChecks.length" class="s-fails">
              <div v-for="item in failedChecks" :key="item.name" class="s-fail">
                <strong>{{ item.name }}</strong>
                <p>{{ item.message }}</p>
              </div>
            </div>

            <div class="actions">
              <n-button type="primary" :loading="loading" @click="emit('health')">{{ t('guide.step.three.healthCheck') }}</n-button>
              <n-button secondary :loading="loading" @click="emit('refresh')">{{ t('console.actions.refresh') }}</n-button>
            </div>

            <div class="cmd">
              <div class="cmd-label">{{ t('guide.step.three.quickVerify') }}</div>
              <div class="mono">访问 {{ props.listenAddress || 'http://127.0.0.1:11434' }}/health</div>
            </div>
          </div>
        </div>

        <!-- Quick Actions -->
        <div class="card">
          <div class="card-header">
            <span class="card-title">{{ t('dashboard.quickActions') }}</span>
          </div>
          <div class="actions">
            <n-button tertiary @click="ui.showSettings = true">{{ t('guide.actions.preferences') }}</n-button>
            <n-button
              tertiary
              type="primary"
              :disabled="!codexBaseURL"
              @click="emit('copy', codexBaseURL)"
            >
              {{ t('guide.actions.copyBaseUrl') }}
            </n-button>
          </div>
        </div>
      </div>
    </div>

    <!-- Add Profile Dialog -->
    <n-modal v-model:show="showAddDialog" preset="dialog" :title="t('models.addProfile')" :positive-text="t('models.confirmAdd')" :negative-text="t('models.cancel')" @positive-click="handleAddProfile">
      <n-form label-placement="top" size="small">
        <n-form-item :label="t('models.profileName')">
          <n-input v-model:value="newProfileName" :placeholder="t('models.profileNamePlaceholder')" />
        </n-form-item>
        <n-form-item :label="t('models.provider')">
          <n-select v-model:value="newProfileProvider" :options="providerOptions" />
        </n-form-item>
        <n-form-item label="API Key">
          <n-input
            v-model:value="newProfileApiKey"
            type="password"
            show-password-on="click"
            :placeholder="getProviderPreset(newProfileProvider)?.placeholderApiKey ?? 'sk-...'"
          />
        </n-form-item>
      </n-form>
    </n-modal>

    <!-- Editor Drawer -->
    <n-drawer v-model:show="showEditor" :width="520" placement="right">
      <n-drawer-content :title="t('models.editor.title')" closable>
        <ModelEditorPanel
          v-if="editingProfileId"
          :profile-id="editingProfileId"
          @save="handleEditorSave"
        />
      </n-drawer-content>
    </n-drawer>

    <!-- Proxy Settings Drawer -->
    <n-drawer v-model:show="showProxySettings" :width="520" placement="right">
      <n-drawer-content :title="t('proxy.title')" closable>
        <ProxySettingsPanel
          :config="store.config"
          @save="showProxySettings = false"
        />
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<style scoped>
.dashboard {
  display: grid;
  gap: 16px;
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

.card-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* Status */
.status-section {
  display: grid;
  gap: 12px;
}

.s-status {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: rgba(11, 18, 32, 0.86);
}

.s-dot {
  width: 10px;
  height: 10px;
  border-radius: 999px;
  background: rgba(11, 18, 32, 0.26);
  box-shadow: 0 0 0 4px rgba(11, 18, 32, 0.06);
  flex-shrink: 0;
}
.s-dot[data-status='running'] {
  background: var(--accent-2);
  box-shadow: 0 0 0 4px rgba(19, 194, 194, 0.16);
}
.s-dot[data-status='starting'] {
  background: var(--warning);
  box-shadow: 0 0 0 4px rgba(216, 150, 20, 0.16);
}
.s-dot[data-status='error'] {
  background: var(--danger);
  box-shadow: 0 0 0 4px rgba(212, 56, 13, 0.16);
}

.s-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 16px;
  font-size: 12px;
}
.s-meta-item {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
}
.s-meta-label {
  color: var(--muted);
  font-size: 11px;
}
.s-meta-item strong {
  font-weight: 600;
  color: rgba(11, 18, 32, 0.9);
  word-break: break-all;
}
.s-meta-item[data-tone='error'] strong {
  color: rgba(212, 56, 13, 0.92);
}

.s-health {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 16px;
  border: 1px solid var(--border);
  background: rgba(255, 255, 255, 0.82);
  font-size: 13px;
  color: rgba(11, 18, 32, 0.86);
}
.h-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: var(--muted);
  flex-shrink: 0;
}
.s-health[data-tone='success'] .h-dot { background: var(--accent-2); }
.s-health[data-tone='warning'] .h-dot { background: var(--warning); }

.s-fails {
  display: grid;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 16px;
  border: 1px solid rgba(216, 150, 20, 0.22);
  background: rgba(255, 255, 255, 0.82);
}
.s-fail {
  display: grid;
  gap: 4px;
}
.s-fail strong {
  font-size: 12px;
  color: rgba(11, 18, 32, 0.9);
}
.s-fail p {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: rgba(11, 18, 32, 0.72);
  word-break: break-word;
}

.actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.cmd {
  padding: 10px 12px;
  border-radius: 16px;
  border: 1px dashed rgba(22, 119, 255, 0.28);
  background: rgba(22, 119, 255, 0.06);
  display: grid;
  gap: 6px;
}

.cmd-label {
  font-size: 12px;
  color: rgba(11, 18, 32, 0.72);
  font-weight: 600;
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  word-break: break-all;
  color: rgba(11, 18, 32, 0.9);
}
</style>
