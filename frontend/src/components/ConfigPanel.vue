<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AppConfig, BridgeStatusPayload } from '../types'
import KeyValueEditor from './KeyValueEditor.vue'
import { maskSecret } from '../utils/format'

const props = defineProps<{
  config: AppConfig
  status: BridgeStatusPayload
  loading: boolean
}>()

const emit = defineEmits<{
  save: [value: AppConfig]
  start: [value: AppConfig]
  stop: []
  restart: []
  copy: [value: string]
}>()

const formValue = ref<AppConfig>({ ...props.config })
const showAdvanced = ref(false)
const { t } = useI18n()

watch(
  () => props.config,
  (value) => {
    formValue.value = {
      ...value,
      mappings: { ...value.mappings },
      headers: { ...value.headers },
    }
  },
  { immediate: true, deep: true },
)

const isRunning = computed(() => props.status.status === 'running')
const maskedApiKey = computed(() => maskSecret(formValue.value.apiKey))
const apiKeyHint = computed(() =>
  t('config.fields.apiKeyHint', {
    masked: maskedApiKey.value || t('config.fields.apiKeyMissing'),
  }),
)

function submitSave() {
  emit('save', formValue.value)
}
</script>

<template>
  <div class="config-panel">
    <div class="panel-head">
      <div>
        <h3>{{ t('config.title') }}</h3>
        <p>{{ t('config.desc') }}</p>
      </div>
    </div>

    <n-form label-placement="top" :model="formValue">
      <div class="form-grid">
        <n-form-item label="DeepSeek Base URL">
          <n-input v-model:value="formValue.deepseekBaseURL" placeholder="https://api.deepseek.com/v1" />
        </n-form-item>
        <n-form-item :label="t('config.fields.defaultModel')">
          <n-input v-model:value="formValue.defaultModel" placeholder="deepseek-chat" />
        </n-form-item>
        <n-form-item label="API Key" class="span-2">
          <n-input
            v-model:value="formValue.apiKey"
            type="password"
            show-password-on="click"
            placeholder="sk-..."
          />
          <div class="field-hint">{{ apiKeyHint }}</div>
        </n-form-item>
        <n-form-item :label="t('config.fields.listenHost')">
          <n-input v-model:value="formValue.listenHost" placeholder="127.0.0.1" />
        </n-form-item>
        <n-form-item :label="t('config.fields.listenPort')">
          <n-input-number v-model:value="formValue.listenPort" :min="1" :max="65535" />
        </n-form-item>
        <n-form-item :label="t('config.fields.requestTimeout')">
          <n-input-number v-model:value="formValue.requestTimeoutMs" :min="1000" :step="1000" />
        </n-form-item>
        <n-form-item :label="t('config.fields.maxRetries')">
          <n-input-number v-model:value="formValue.maxRetries" :min="0" :max="5" />
        </n-form-item>
      </div>
    </n-form>

    <div class="action-bar">
      <n-space>
        <n-button secondary :loading="loading" @click="submitSave">{{ t('config.actions.save') }}</n-button>
        <n-button type="primary" :disabled="isRunning" :loading="loading" @click="emit('start', formValue)">
          {{ t('config.actions.start') }}
        </n-button>
        <n-button secondary :disabled="!isRunning" :loading="loading" @click="emit('restart')">
          {{ t('config.actions.restart') }}
        </n-button>
        <n-button tertiary type="error" :disabled="!isRunning" :loading="loading" @click="emit('stop')">
          {{ t('config.actions.stop') }}
        </n-button>
      </n-space>
      <n-button
        tertiary
        :disabled="!status.listenAddress"
        @click="emit('copy', status.listenAddress)"
      >
        {{ t('config.actions.copyLocal') }}
      </n-button>
    </div>

    <n-collapse-transition :show="showAdvanced">
      <div class="advanced-panel">
        <KeyValueEditor
          v-model:model-value="formValue.mappings"
          :title="t('config.advanced.modelMapping.title')"
          :description="t('config.advanced.modelMapping.desc')"
          :key-placeholder="t('config.advanced.modelMapping.keyPlaceholder')"
          :value-placeholder="t('config.advanced.modelMapping.valuePlaceholder')"
        />
        <KeyValueEditor
          v-model:model-value="formValue.headers"
          :title="t('config.advanced.headers.title')"
          :description="t('config.advanced.headers.desc')"
          :key-placeholder="t('config.advanced.headers.keyPlaceholder')"
          :value-placeholder="t('config.advanced.headers.valuePlaceholder')"
        />
      </div>
    </n-collapse-transition>

    <n-button text type="primary" @click="showAdvanced = !showAdvanced">
      {{ showAdvanced ? t('config.actions.collapseAdvanced') : t('config.actions.expandAdvanced') }}
    </n-button>
  </div>
</template>

<style scoped>
.config-panel {
  display: grid;
  gap: 18px;
  padding: 20px;
  border-radius: 22px;
  border: 1px solid var(--border);
  background: var(--surface);
  box-shadow: 0 12px 34px rgba(14, 30, 68, 0.08);
}

.panel-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.panel-head h3 {
  margin: 0 0 6px;
  font-size: 17px;
  color: var(--text);
}

.panel-head p {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--muted);
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 16px;
}

.span-2 {
  grid-column: span 2;
}

.field-hint {
  margin-top: 8px;
  font-size: 12px;
  color: var(--muted);
}

.action-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.advanced-panel {
  display: grid;
  gap: 20px;
  padding-top: 8px;
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
