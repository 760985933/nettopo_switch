<script setup lang="ts">
import { ref } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { getProviderPreset } from '../utils/providers'
import ModelEditorPanel from '../components/ModelEditorPanel.vue'
import ProfileList from '../components/ProfileList.vue'

const store = useAppStore()
const message = useMessage()
const { t } = useI18n()

const showAddDialog = ref(false)
const newProfileName = ref('')
const newProfileProvider = ref('deepseek')
const newProfileApiKey = ref('')
const editingProfileId = ref<string | null>(null)
const showEditor = ref(false)

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
  await store.addProfile(newProfileName.value.trim(), newProfileProvider.value, undefined, newProfileApiKey.value || undefined)
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
        @edit="handleEdit"
        @delete="handleDelete"
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
