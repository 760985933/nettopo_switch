<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { getProviderPreset } from '../utils/providers'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import ModelEditorPanel from '../components/ModelEditorPanel.vue'
import ProfileList from '../components/ProfileList.vue'

const store = useAppStore()
const message = useMessage()
const { t } = useI18n()

const showAddDialog = ref(false)
const newProfileName = ref('')
const newProfileProvider = ref('deepseek')
const newProfileApiKey = ref('')
const billingMode = ref<'paygo' | 'tokenplan'>('paygo')
const editingProfileId = ref<string | null>(null)
const showEditor = ref(false)

const hasTokenPlan = computed(() => {
  const p = getProviderPreset(newProfileProvider.value)
  return !!(p?.tokenPlanOpenAIBaseURL || p?.tokenPlanAnthropicBaseURL)
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

async function handleAdd() {
  if (!newProfileName.value.trim()) return
  const preset = getProviderPreset(newProfileProvider.value)
  const isMessages = preset?.apiType === 'messages'
  const isTokenPlan = billingMode.value === 'tokenplan'
  let baseURL: string | undefined
  if (preset) {
    if (isTokenPlan) {
      baseURL = isMessages && preset.tokenPlanAnthropicBaseURL
        ? preset.tokenPlanAnthropicBaseURL
        : preset.tokenPlanOpenAIBaseURL ?? preset.defaultBaseURL
    } else {
      baseURL = isMessages && preset.anthropicBaseURL
        ? preset.anthropicBaseURL
        : preset.defaultBaseURL
    }
  }
  await store.addProfile(newProfileName.value.trim(), newProfileProvider.value, undefined, newProfileApiKey.value || undefined, baseURL)
  newProfileName.value = ''
  newProfileProvider.value = 'deepseek'
  newProfileApiKey.value = ''
  showAddDialog.value = false
  message.success(t('models.toast.added'))
}

function handleEdit(id: string) {
  editingProfileId.value = id
  showEditor.value = true
}

function handleDelete(id: string) {
  store.deleteProfile(id)
}

function handleEditorSave() {
  showEditor.value = false
  editingProfileId.value = null
}
</script>

<template>
  <div class="models-page">
    <div class="page-header">
      <div>
        <h2>{{ t('models.title') }}</h2>
        <p class="page-desc">{{ t('models.description') }}</p>
      </div>
      <n-button type="primary" size="small" @click="showAddDialog = true">
        {{ t('models.addProfile') }}
      </n-button>
    </div>

    <div class="card">
      <div class="card-header">
        <span class="card-title">{{ t('models.title') }}</span>
      </div>
      <ProfileList
        :profiles="store.profileList"
        :current-profile-id="store.config.currentProfileId"
        :loading="store.isBusy"
        sortable
        @edit="handleEdit"
        @delete="handleDelete"
        @reorder="(ids) => store.reorderAllProfiles(ids)"
      />
    </div>

    <!-- Add Profile Dialog -->
    <n-modal v-model:show="showAddDialog" preset="dialog" :title="t('models.addProfile')" :positive-text="t('models.confirmAdd')" :negative-text="t('models.cancel')" @positive-click="handleAdd">
      <n-form label-placement="top" size="small">
        <n-form-item :label="t('models.profileName')">
          <n-input v-model:value="newProfileName" :placeholder="t('models.profileNamePlaceholder')" />
        </n-form-item>
        <n-form-item :label="t('models.provider')">
          <n-select v-model:value="newProfileProvider" :options="providerOptions" />
        </n-form-item>
        <n-form-item v-if="hasTokenPlan" :label="t('models.billingMode')">
          <n-radio-group v-model:value="billingMode" size="small">
            <n-radio value="paygo">{{ t('models.billingPaygo') }}</n-radio>
            <n-radio value="tokenplan">{{ t('models.billingTokenplan') }}</n-radio>
          </n-radio-group>
        </n-form-item>
        <n-form-item label="API Key">
          <div style="display:flex;gap:4px;align-items:center;width:100%">
            <n-input
              v-model:value="newProfileApiKey"
              type="password"
              show-password-on="click"
              :placeholder="getProviderPreset(newProfileProvider)?.placeholderApiKey ?? 'sk-...'"
              style="flex:1;min-width:0"
            />
            <n-button
              v-if="getProviderPreset(newProfileProvider)?.apiKeyURL"
              text
              size="small"
              @click="BrowserOpenURL(getProviderPreset(newProfileProvider)!.apiKeyURL)"
            >
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
            </n-button>
          </div>
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
  </div>
</template>

<style scoped>
.models-page {
  display: grid;
  gap: 20px;
  max-width: 720px;
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
</style>
