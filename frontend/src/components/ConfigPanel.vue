<script setup lang="ts">
import { useMessage } from 'naive-ui'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import type { Profile } from '../types'
import KeyValueEditor from './KeyValueEditor.vue'
import { maskSecret } from '../utils/format'
import { PROVIDER_PRESETS, getProviderPreset } from '../utils/providers'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

const store = useAppStore()
const props = defineProps<{
  profileId?: string
}>()
const emit = defineEmits<{
  save: []
}>()

const formProfile = ref<Profile>({} as Profile)
const showAdvanced = ref(true)
const { t } = useI18n()
const message = useMessage()

const editingProfile = computed(() => {
  const id = props.profileId ?? store.config.currentProfileId
  return store.config.profiles[id] ?? null
})

const providerOptions = PROVIDER_PRESETS.map((p) => ({
  label: p.label,
  value: p.id,
}))

// Sync form when switching profiles
watch(
  () => props.profileId ?? store.config.currentProfileId,
  () => syncForm(),
)
watch(
  () => store.config.profiles,
  () => syncForm(),
  { deep: true },
)

function syncForm() {
  const p = editingProfile.value
  if (p) {
    formProfile.value = {
      ...p,
      mappings: { ...p.mappings },
      headers: { ...p.headers },
    }
  }
}
syncForm()

const maskedApiKey = computed(() => maskSecret(formProfile.value.apiKey))
const apiKeyHint = computed(() =>
  t('config.fields.apiKeyHint', {
    masked: maskedApiKey.value || t('config.fields.apiKeyMissing'),
  }),
)

function onProviderChange(providerId: string) {
  const preset = getProviderPreset(providerId)
  if (preset && providerId !== 'custom') {
    formProfile.value.baseURL = preset.defaultBaseURL
    formProfile.value.defaultModel = preset.defaultModel
  }
}

async function submitSave() {
  const id = props.profileId ?? store.config.currentProfileId
  const profiles = { ...store.config.profiles }
  profiles[id] = { ...formProfile.value }
  // If editing a different profile, also switch active profile
  const updated = {
    ...store.config,
    currentProfileId: id,
    profiles,
  }
  await store.saveConfig(updated)
  if (store.isRunning) {
    try {
      await store.restartProxy()
      message.success(t('config.toast.savedAndRestarted', { name: formProfile.value.name }))
    } catch (e) {
      message.warning(t('config.toast.savedRestartFailed', { error: String(e) }))
    }
  }
  emit('save')
}
</script>

<template>
  <div class="config-panel">
    <div class="panel-head">
      <div>
        <h3>{{ t('config.title') }}</h3>
        <p v-if="editingProfile">
          {{ t('config.editing') }}: <strong>{{ editingProfile.name }}</strong>
        </p>
        <p v-else>{{ t('config.noProfile') }}</p>
      </div>
    </div>

    <n-form label-placement="top" :model="formProfile" size="small">
      <div class="form-grid">
        <n-form-item :label="t('config.fields.profileName')">
          <n-input v-model:value="formProfile.name" size="small" />
        </n-form-item>
        <n-form-item label="提供商">
          <div class="provider-row">
            <n-select
              v-model:value="formProfile.provider"
              :options="providerOptions"
              size="small"
              @update:value="onProviderChange"
            />
            <n-button
              v-if="getProviderPreset(formProfile.provider)?.docsURL"
              text
              size="small"
              @click="BrowserOpenURL(getProviderPreset(formProfile.provider)!.docsURL)"
            >
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
            </n-button>
          </div>
        </n-form-item>
        <n-form-item :label="t('config.fields.defaultModel')">
          <n-input
            v-model:value="formProfile.defaultModel"
            :placeholder="getProviderPreset(formProfile.provider)?.placeholderModel ?? 'gpt-4o'"
            size="small"
          />
        </n-form-item>
        <n-form-item label="API Base URL" class="span-2">
          <n-input
            v-model:value="formProfile.baseURL"
            :placeholder="getProviderPreset(formProfile.provider)?.defaultBaseURL ?? 'https://api.deepseek.com/v1'"
            size="small"
          />
        </n-form-item>
        <n-form-item label="API Key" class="span-2" required>
          <n-input
            v-model:value="formProfile.apiKey"
            type="password"
            show-password-on="click"
            :placeholder="getProviderPreset(formProfile.provider)?.placeholderApiKey ?? 'sk-...'"
            size="small"
          />
          <div class="field-hint">{{ apiKeyHint }}</div>
        </n-form-item>
        <n-form-item :label="t('config.fields.listenHost')">
          <n-input v-model:value="store.config.listenHost" placeholder="127.0.0.1" size="small" />
        </n-form-item>
        <n-form-item :label="t('config.fields.listenPort')">
          <n-input-number v-model:value="store.config.listenPort" :min="1" :max="65535" size="small" />
        </n-form-item>
        <n-form-item :label="t('config.fields.requestTimeout')">
          <n-input-number v-model:value="formProfile.requestTimeoutMs" :min="1000" :step="1000" size="small" />
        </n-form-item>
        <n-form-item :label="t('config.fields.maxRetries')">
          <n-input-number v-model:value="formProfile.maxRetries" :min="0" :max="5" size="small" />
        </n-form-item>
      </div>
    </n-form>

    <div class="action-bar">
      <n-button type="primary" :loading="store.isBusy" @click="submitSave">
        {{ t('config.actions.save') }}
      </n-button>
    </div>

    <div v-if="showAdvanced" class="advanced-panel">
        <KeyValueEditor
          v-model:model-value="formProfile.mappings"
          :title="t('config.advanced.modelMapping.title')"
          :description="t('config.advanced.modelMapping.desc')"
          :key-placeholder="t('config.advanced.modelMapping.keyPlaceholder')"
          :value-placeholder="t('config.advanced.modelMapping.valuePlaceholder')"
          size="small"
        />
        <KeyValueEditor
          v-model:model-value="formProfile.headers"
          :title="t('config.advanced.headers.title')"
          :description="t('config.advanced.headers.desc')"
          :key-placeholder="t('config.advanced.headers.keyPlaceholder')"
          :value-placeholder="t('config.advanced.headers.valuePlaceholder')"
          size="small"
        />
      </div>

    <n-button text type="primary" @click="showAdvanced = !showAdvanced">
      {{ showAdvanced ? t('config.actions.collapseAdvanced') : t('config.actions.expandAdvanced') }}
    </n-button>
  </div>
</template>

<style scoped>
.config-panel {
  display: grid;
  gap: 8px;
  padding: 16px;
  border-radius: 22px;
  border: 1px solid var(--border);
  background: var(--surface);
  box-shadow: 0 10px 30px rgba(14, 30, 68, 0.08);
}

.panel-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.panel-head h3 {
  margin: 0 0 2px;
  font-size: 15px;
  color: var(--text);
}

.panel-head p {
  margin: 0;
  font-size: 11px;
  line-height: 1.5;
  color: var(--muted);
}

.panel-head strong {
  color: var(--text);
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 2px 12px;
}

.span-2 {
  grid-column: span 2;
}

.field-hint {
  margin-top: 4px;
  font-size: 11px;
  color: var(--muted);
}

.provider-row {
  display: flex;
  gap: 4px;
  align-items: center;
  width: 100%;
}

.provider-row .n-select {
  flex: 1;
  min-width: 0;
}

.action-bar {
  display: flex;
  align-items: center;
  gap: 8px;
}

.advanced-panel {
  display: grid;
  gap: 14px;
  padding-top: 4px;
}

@media (max-width: 920px) {
  .panel-head,
  .action-bar {
    flex-direction: column;
    align-items: stretch;
  }

  .form-grid {
    grid-template-columns: 1fr;
  }

  .span-2 {
    grid-column: span 1;
  }
}
</style>
