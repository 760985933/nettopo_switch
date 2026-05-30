<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import type { SourceID, InstanceConfig } from '../types'
import KeyValueEditor from './KeyValueEditor.vue'

const store = useAppStore()
const message = useMessage()
const { t } = useI18n()

const props = withDefaults(defineProps<{
  source?: SourceID
  config: typeof store.config
}>(), {
  source: 'codex',
})

const emit = defineEmits<{
  save: []
}>()

// Derive local state from instance config, falling back to flat fields
function instanceFromConfig(): InstanceConfig {
  const ic = props.config.instances?.[props.source]
  if (ic) return { ...ic, headers: { ...ic.headers } }
  return {
    listenHost: props.config.listenHost || '127.0.0.1',
    listenPort: props.config.listenPort || 17419,
    requestTimeoutMs: props.config.requestTimeoutMs || 60000,
    maxRetries: props.config.maxRetries ?? 3,
    mappings: { ...props.config.mappings },
    headers: { ...props.config.headers },
    currentProfileId: props.config.currentProfileId || '',
  }
}

const localInst = ref<InstanceConfig>(instanceFromConfig())

watch(
  () => [props.config, props.source],
  () => {
    localInst.value = instanceFromConfig()
  },
  { deep: true, immediate: true },
)

const proxyAddress = computed(() => {
  const host = localInst.value.listenHost || '127.0.0.1'
  const port = localInst.value.listenPort || 17419
  return `http://${host}:${port}/v1`
})

async function handleSave() {
  const existing = props.config.instances?.[props.source]
  const merged: InstanceConfig = {
    ...(existing || makeFallbackInst()),
    ...localInst.value,
  }
  const updated = {
    ...props.config,
    instances: {
      ...props.config.instances,
      [props.source]: merged,
    },
  }
  await store.saveConfig(updated as any)
  message.success(t('proxy.toast.saved'))
  emit('save')
}

function makeFallbackInst(): InstanceConfig {
  return {
    listenHost: '127.0.0.1',
    listenPort: props.source === 'claude' ? 17420 : 17419,
    requestTimeoutMs: 60000,
    maxRetries: 3,
    mappings: {},
    headers: {},
    currentProfileId: props.config.currentProfileId || 'default',
  }
}
</script>

<template>
  <div class="proxy-panel">
    <!-- Network -->
    <div class="card">
      <div class="card-header">
        <span class="card-title">{{ t('proxy.section.network') }}</span>
      </div>
      <n-form label-placement="top" size="small">
        <div class="form-grid">
          <n-form-item :label="t('proxy.listenHost')">
            <n-input v-model:value="localInst.listenHost" placeholder="127.0.0.1" />
          </n-form-item>
          <n-form-item :label="t('proxy.listenPort')">
            <n-input-number v-model:value="localInst.listenPort" :min="1" :max="65535" style="width:100%" />
          </n-form-item>
        </div>
        <div class="address-hint">
          <div class="hint-label">{{ t('proxy.proxyAddress') }}</div>
          <code>{{ proxyAddress }}</code>
        </div>
      </n-form>
    </div>

    <!-- Request Behavior -->
    <div class="card">
      <div class="card-header">
        <span class="card-title">{{ t('proxy.section.transport') }}</span>
      </div>
      <n-form label-placement="top" size="small">
        <div class="form-grid">
          <n-form-item :label="t('proxy.requestTimeout')">
            <n-input-number v-model:value="localInst.requestTimeoutMs" :min="1000" :step="1000" style="width:100%" />
          </n-form-item>
          <n-form-item :label="t('proxy.maxRetries')">
            <n-input-number v-model:value="localInst.maxRetries" :min="0" :max="5" style="width:100%" />
          </n-form-item>
        </div>
      </n-form>
    </div>

    <!-- Custom Headers -->
    <div class="card">
      <div class="card-header">
        <span class="card-title">{{ t('proxy.section.headers') }}</span>
      </div>
      <KeyValueEditor
        v-model:model-value="localInst.headers"
        :title="t('proxy.customHeaders')"
        :description="t('proxy.customHeadersDesc')"
        :key-placeholder="t('proxy.headerKeyPlaceholder')"
        :value-placeholder="t('proxy.headerValuePlaceholder')"
      />
    </div>

    <div class="action-bar">
      <n-button type="primary" :loading="store.isBusy" @click="handleSave">
        {{ t('proxy.save') }}
      </n-button>
    </div>
  </div>
</template>

<style scoped>
.proxy-panel {
  display: grid;
  gap: 16px;
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

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
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

.address-hint code {
  font-size: 13px;
  font-weight: 600;
  user-select: all;
  color: rgba(11, 18, 32, 0.9);
}

.action-bar {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
