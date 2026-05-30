<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import type { Profile } from '../types'
import KeyValueEditor from './KeyValueEditor.vue'
import { maskSecret } from '../utils/format'
import { PROVIDER_PRESETS, getProviderPreset, BILLING_MODE_LABELS } from '../utils/providers'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

const store = useAppStore()
const props = defineProps<{
  profileId: string
}>()
const emit = defineEmits<{
  save: []
}>()

const formProfile = ref<Profile>({} as Profile)
const billingMode = ref<'paygo' | 'tokenplan'>('paygo')
const { t } = useI18n()
const message = useMessage()

const editingProfile = computed(() => {
  return store.config.profiles[props.profileId] ?? null
})

const providerOptions = PROVIDER_PRESETS.map((p) => ({
  label: p.label,
  value: p.id,
}))

const hasTokenPlan = computed(() => {
  const p = getProviderPreset(formProfile.value.provider)
  return !!(p?.tokenPlanOpenAIBaseURL || p?.tokenPlanAnthropicBaseURL)
})

const apiTypeOptions = [
  { label: 'Chat Completions（OpenAI 兼容）', value: 'chat_completions' },
  { label: 'Responses（Codex / OpenAI）', value: 'responses' },
  { label: 'Messages（Anthropic Claude）', value: 'messages' },
  { label: 'Google（Gemini）', value: 'google' },
]

watch(
  () => props.profileId,
  () => syncForm(),
  { immediate: true },
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
      apiType: p.apiType || getProviderPreset(p.provider)?.apiType || 'chat_completions',
    }
    const preset = getProviderPreset(p.provider)
    billingMode.value = preset?.tokenPlanOpenAIBaseURL === p.baseURL || preset?.tokenPlanAnthropicBaseURL === p.baseURL
      ? 'tokenplan'
      : 'paygo'
  }
}

const maskedApiKey = computed(() => maskSecret(formProfile.value.apiKey))
const apiKeyHint = computed(() =>
  t('config.fields.apiKeyHint', {
    masked: maskedApiKey.value || t('config.fields.apiKeyMissing'),
  }),
)

function onProviderChange(providerId: string) {
  const preset = getProviderPreset(providerId)
  billingMode.value = 'paygo'
  if (preset && providerId !== 'custom') {
    formProfile.value.baseURL = preset.defaultBaseURL
    formProfile.value.defaultModel = preset.defaultModel
    formProfile.value.apiType = preset.apiType
  }
}

function onBillingModeChange(mode: 'paygo' | 'tokenplan') {
  billingMode.value = mode
  const preset = getProviderPreset(formProfile.value.provider)
  if (!preset) return
  const isAnthropic = formProfile.value.apiType === 'messages'
  if (mode === 'tokenplan') {
    formProfile.value.baseURL = isAnthropic && preset.tokenPlanAnthropicBaseURL
      ? preset.tokenPlanAnthropicBaseURL
      : preset.tokenPlanOpenAIBaseURL ?? preset.defaultBaseURL
  } else {
    formProfile.value.baseURL = isAnthropic && preset.anthropicBaseURL
      ? preset.anthropicBaseURL
      : preset.defaultBaseURL
  }
}

async function submitSave() {
  const profiles = { ...store.config.profiles }
  profiles[props.profileId] = { ...formProfile.value }
  const updated = {
    ...store.config,
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
  <div class="model-editor">
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
        <n-form-item v-if="hasTokenPlan" :label="t('models.billingMode')" class="span-2">
          <n-radio-group v-model:value="billingMode" size="small" @update:value="onBillingModeChange">
            <n-radio value="paygo">{{ BILLING_MODE_LABELS.paygo }}</n-radio>
            <n-radio value="tokenplan">{{ BILLING_MODE_LABELS.tokenplan }}</n-radio>
          </n-radio-group>
        </n-form-item>
        <n-form-item label="API 格式" class="span-2">
          <n-select
            v-model:value="formProfile.apiType"
            :options="apiTypeOptions"
            size="small"
          />
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
      </div>
    </n-form>

    <div class="action-bar">
      <n-button type="primary" :loading="store.isBusy" @click="submitSave">
        {{ t('config.actions.save') }}
      </n-button>
    </div>

    <div class="advanced-section">
      <KeyValueEditor
        v-model:model-value="formProfile.mappings"
        :title="t('config.advanced.modelMapping.title')"
        :description="t('config.advanced.modelMapping.desc')"
        :key-placeholder="t('config.advanced.modelMapping.keyPlaceholder')"
        :value-placeholder="t('config.advanced.modelMapping.valuePlaceholder')"
        size="small"
      />
    </div>
  </div>
</template>

<style scoped>
.model-editor {
  display: grid;
  gap: 16px;
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

.advanced-section {
  display: grid;
  gap: 14px;
  padding-top: 4px;
}

@media (max-width: 920px) {
  .form-grid {
    grid-template-columns: 1fr;
  }

  .span-2 {
    grid-column: span 1;
  }
}
</style>
