<script setup lang="ts">
import { ref, computed } from 'vue'
import { useMessage, useDialog } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '../stores/app'
import { useCodexStore } from '../stores/codex'
import { useUiStore } from '../stores/ui'
import { getProviderPreset, getClaudeBaseMappings } from '../utils/providers'
import ProfileList from './ProfileList.vue'
import ModelEditorPanel from './ModelEditorPanel.vue'
import ProxySettingsPanel from './ProxySettingsPanel.vue'
import ProxyStatusCard from './ProxyStatusCard.vue'
import CodexLoginActions from './CodexLoginActions.vue'
import type { ProxyStatusPayload, HealthCheckResult, SourceID, Profile } from '../types'

const emit = defineEmits<{
  copy: [value: string]
  health: []
  stop: []
  refresh: []
}>()

const props = defineProps<{
  source: SourceID
  listenAddress: string
  loading: boolean
  status: ProxyStatusPayload
  health: HealthCheckResult | null
}>()

const store = useAppStore()
const codexStore = useCodexStore()
const ui = useUiStore()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const { t } = useI18n()

const codexBaseURL = computed(() => {
  if (!props.listenAddress) return ''
  return props.listenAddress.replace(/\/+$/, '') + '/v1'
})

const currentProfile = computed(() => store.currentProfile)

// Instance-aware proxy profiles for this source
const proxyProfilesForSource = computed(() => {
  const ic = store.instanceConfig(props.source)
  const ids = ic?.proxyProfileIds || []
  return ids.map(id => store.config.profiles[id]).filter(Boolean) as typeof store.proxyProfiles
})

// ── Add proxy entry ──
const showAddDialog = ref(false)
const addMode = ref<'new' | 'link'>('new')
const newProfileName = ref('')
const newProfileProvider = ref('deepseek')
const newProfileApiKey = ref('')
const linkProfileId = ref<string | null>(null)
const billingMode = ref<'paygo' | 'tokenplan'>('paygo')

const unlinkedProfiles = computed(() => {
  const proxyIds = new Set(proxyProfilesForSource.value.map(p => p.id))
  return store.profileList.filter(p => !proxyIds.has(p.id))
})

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

const linkProfileOptions = computed(() =>
  unlinkedProfiles.value.map(p => ({ label: `${p.name} (${p.provider})`, value: p.id }))
)

const hasTokenPlan = computed(() => {
  const p = getProviderPreset(newProfileProvider.value)
  return !!(p?.tokenPlanOpenAIBaseURL || p?.tokenPlanAnthropicBaseURL)
})

function openAddDialog() {
  addMode.value = 'new'
  newProfileName.value = ''
  newProfileProvider.value = 'deepseek'
  newProfileApiKey.value = ''
  linkProfileId.value = null
  billingMode.value = 'paygo'
  showAddDialog.value = true
}

async function handleAddProxy() {
  if (addMode.value === 'new') {
    if (!newProfileName.value.trim()) return
    const preset = getProviderPreset(newProfileProvider.value)
    if (!preset) {
      message.warning(t('models.invalidProvider'))
      return
    }
    const isClaude = props.source === 'claude'
    const isTokenPlan = billingMode.value === 'tokenplan'
    let baseURL: string
    let defaultModel: string
    if (isTokenPlan) {
      baseURL = isClaude && preset.tokenPlanAnthropicBaseURL
        ? preset.tokenPlanAnthropicBaseURL
        : preset.tokenPlanOpenAIBaseURL ?? preset.defaultBaseURL
      defaultModel = preset.defaultModel
    } else {
      baseURL = isClaude && preset.anthropicBaseURL
        ? preset.anthropicBaseURL
        : preset.defaultBaseURL
      defaultModel = isClaude && preset.anthropicModel
        ? preset.anthropicModel
        : preset.defaultModel
    }
    const apiType = isClaude ? 'messages' : preset.apiType
    const profile: Profile = {
      id: '',
      name: newProfileName.value.trim(),
      provider: newProfileProvider.value,
      baseURL,
      apiKey: newProfileApiKey.value || '',
      defaultModel,
      apiType,
      mappings: isClaude ? getClaudeBaseMappings(preset) : {},
    }
    const newId = 'profile_' + Date.now().toString(36)
    const newInst = store.instanceConfig(props.source)
    const newUpdated: any = {
      ...store.config,
      profiles: {
        ...store.config.profiles,
        [newId]: { ...profile, id: newId },
      },
    }
    newUpdated.instances = {
      ...store.config.instances,
      [props.source]: {
        ...newInst,
        proxyProfileIds: [...(newInst.proxyProfileIds || []), newId],
        currentProfileId: newInst.currentProfileId || newId,
      },
    }
    await store.saveConfig(newUpdated as any)
    message.success(t('models.toast.added'))
  } else {
    if (!linkProfileId.value) return
    const ic = store.instanceConfig(props.source)
    const ids = [...(ic.proxyProfileIds || []), linkProfileId.value]
    const updated: any = {
      ...store.config,
      instances: {
        ...store.config.instances,
        [props.source]: { ...ic, proxyProfileIds: ids },
      },
    }
    await store.saveConfig(updated as any)
    message.success(t('dashboard.linkedProxy'))
  }
  showAddDialog.value = false
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

async function handleRestoreCodex() {
  try {
    const path = await codexStore.restoreCodexConfigToml()
    message.success(t('settings.toast.restored', { path: path || '' }))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  }
}

const enableClaudeLoadingId = ref<string | null>(null)
async function handleEnableClaude(profileId: string) {
  enableClaudeLoadingId.value = profileId
  try {
    const ic = store.instanceConfig('claude')
    if (ic && ic.currentProfileId !== profileId) {
      const updated: any = {
        ...store.config,
        instances: {
          ...store.config.instances,
          claude: {
            ...ic,
            currentProfileId: profileId,
            proxyProfileIds: [...(ic.proxyProfileIds || []), profileId].filter((v: string, i: number, a: string[]) => a.indexOf(v) === i),
          },
        },
      }
      await store.saveConfig(updated)
    }
    if (!store.isRunningForSource('claude')) {
      await store.startProxyForSource('claude')
    }
    const { EnableClaudeSettings } = await import('../../wailsjs/go/main/App')
    const path = await EnableClaudeSettings(profileId)
    message.success(t('guide.actions.enableClaudeSuccess', { path: path || '' }))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    enableClaudeLoadingId.value = null
  }
}


async function handleRemoveProxy(id: string) {
  dialog.warning({
    title: t('dashboard.removeProxy'),
    content: t('dashboard.removeProxyConfirm'),
    positiveText: t('common.delete'),
    negativeText: t('models.cancel'),
    onPositiveClick: async () => {
      const ic = store.instanceConfig(props.source)
      const ids = (ic.proxyProfileIds || []).filter(i => i !== id)
      const updated = {
        ...store.config,
        instances: {
          ...store.config.instances,
          [props.source]: { ...ic, proxyProfileIds: ids },
        },
      }
      await store.saveConfig(updated as any)
      message.success(t('dashboard.removedProxy'))
    },
  })
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
            <n-button size="small" @click="openAddDialog">
              <template #icon>
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
              </template>
              {{ t('dashboard.addProxy') }}
            </n-button>
            <n-button text size="small" type="primary" @click="router.push('/models')">
              {{ t('dashboard.manageModels') }}
            </n-button>
          </div>
        </div>

        <ProfileList
          :profiles="proxyProfilesForSource"
          :current-profile-id="store.instanceConfig(source)?.currentProfileId || ''"
          :loading="store.isBusy"
          :show-delete="false"
          sortable
          @edit="handleEdit"
          @delete="handleDelete"
          @select="store.setCurrentProfile"
          @reorder="store.reorderProfiles"
        >
          <template #actions="{ profile }">
            <CodexLoginActions v-if="source === 'codex'" :profile-id="profile.id" />
            <n-button
              v-if="source === 'claude' && store.isRunningForSource('claude') && store.instanceConfig('claude').currentProfileId === profile.id"
              size="small"
              type="error"
              @click.stop="store.stopProxyForSource('claude')"
            >
              {{ t('guide.actions.stop') }}
            </n-button>
            <n-button
              v-if="source === 'claude' && !(store.isRunningForSource('claude') && store.instanceConfig('claude').currentProfileId === profile.id)"
              size="small"
              type="primary"
              :loading="enableClaudeLoadingId === profile.id"
              @click.stop="handleEnableClaude(profile.id)"
            >
              {{ t('guide.actions.enableClaude') }}
            </n-button>
          </template>
          <template #actions-after="{ profile }">
            <n-button size="small" tertiary type="warning" @click="handleRemoveProxy(profile.id)">
              {{ t('dashboard.removeProxy') }}
            </n-button>
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
          <ProxyStatusCard
            :source="source"
            :status="status"
            :loading="loading"
            :health="health"
            @health="emit('health')"
            @refresh="emit('refresh')"
          />

          <div class="cmd">
            <div class="cmd-label">{{ t('guide.step.three.quickVerify') }}</div>
            <div class="mono">访问 {{ props.listenAddress || 'http://127.0.0.1:11434' }}/health</div>
          </div>
        </div>

        <!-- Quick Actions -->
        <div class="card">
          <div class="card-header">
            <span class="card-title">{{ t('dashboard.quickActions') }}</span>
          </div>
          <div class="actions">
            <n-button v-if="source === 'codex'" tertiary @click="ui.openSettings(source)">{{ t('guide.actions.preferences') }}</n-button>
            <n-button v-if="source === 'codex'" tertiary @click="handleRestoreCodex">{{ t('guide.actions.restoreDefault') }}</n-button>
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

    <!-- Add Proxy Dialog -->
    <n-modal v-model:show="showAddDialog" preset="dialog" :title="t('dashboard.addProxy')" :positive-text="t('models.confirmAdd')" :negative-text="t('models.cancel')" @positive-click="handleAddProxy">
      <n-form label-placement="top" size="small">
        <n-form-item :label="t('dashboard.addMode')">
          <n-radio-group v-model:value="addMode">
            <n-radio value="new">{{ t('dashboard.addModeNew') }}</n-radio>
            <n-radio value="link">{{ t('dashboard.addModeLink') }}</n-radio>
          </n-radio-group>
        </n-form-item>

        <template v-if="addMode === 'new'">
          <n-form-item :label="t('models.profileName')">
            <n-input v-model:value="newProfileName" :placeholder="t('models.profileNamePlaceholder')" />
          </n-form-item>
          <n-form-item :label="t('models.provider')">
            <n-select v-model:value="newProfileProvider" :options="providerOptions" />
          </n-form-item>
          <n-form-item v-if="hasTokenPlan" :label="t('models.billingMode')">
            <n-radio-group v-model:value="billingMode">
              <n-radio value="paygo">{{ t('models.billingPaygo') }}</n-radio>
              <n-radio value="tokenplan">{{ t('models.billingTokenplan') }}</n-radio>
            </n-radio-group>
          </n-form-item>
          <n-form-item label="API Key">
            <n-input
              v-model:value="newProfileApiKey"
              type="password"
              show-password-on="click"
              :placeholder="getProviderPreset(newProfileProvider)?.placeholderApiKey ?? 'sk-...'"
            />
          </n-form-item>
        </template>

        <template v-else>
          <n-form-item :label="t('dashboard.linkExisting')">
            <n-select
              v-model:value="linkProfileId"
              :options="linkProfileOptions"
              :placeholder="t('dashboard.linkExistingPlaceholder')"
            />
          </n-form-item>
          <n-empty v-if="unlinkedProfiles.length === 0" :description="t('dashboard.noUnlinked')" style="padding:12px 0" />
        </template>
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
          :source="source"
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
