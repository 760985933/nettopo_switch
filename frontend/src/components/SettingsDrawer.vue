<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import type { AppConfig } from '../types'
import { useAppStore } from '../stores/app'
import { useUiStore } from '../stores/ui'

const props = defineProps<{
  modelValue: boolean
  config: AppConfig
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  save: [value: AppConfig]
  export: []
  codexCopy: []
  codexWrite: []
  codexWriteProfiles: []
}>()

const localConfig = ref<AppConfig>({ ...props.config })
const store = useAppStore()
const ui = useUiStore()
const message = useMessage()
const dialog = useDialog()
const { t } = useI18n()

const codexPath = ref('')
const codexRaw = ref('')
const codexBusy = ref(false)
const codexBackups = ref<string[]>([])
const selectedBackup = ref<string>('')

const backupOptions = computed(() => {
  return codexBackups.value.map((p) => ({
    label: p.split('/').slice(-1)[0] || p,
    value: p,
  }))
})

const needsWireApiFix = computed(() => {
  if (!codexRaw.value) return false
  const value = codexRaw.value
  const providerBlock = /\[\s*model_providers\.Local\s*\][\s\S]*?(\n\[|$)/.exec(value)
  if (!providerBlock) return false
  return /wire_api\s*=\s*"chat"/.test(providerBlock[0])
})

watch(
  () => props.config,
  (value) => {
    localConfig.value = {
      ...value,
      mappings: { ...value.mappings },
      headers: { ...value.headers },
    }
  },
  { deep: true, immediate: true },
)

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      void loadCodexRaw()
    }
  },
)

function submit() {
  emit('save', localConfig.value)
}

async function loadCodexRaw() {
  codexBusy.value = true
  try {
    codexPath.value = await store.getCodexConfigPath()
    codexRaw.value = await store.readCodexConfigToml()
    await refreshCodexBackups()
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    codexBusy.value = false
  }
}

async function refreshCodexBackups() {
  codexBackups.value = await store.listCodexConfigBackups()
  if (selectedBackup.value && !codexBackups.value.includes(selectedBackup.value)) {
    selectedBackup.value = ''
  }
}

async function generateCodexRaw() {
  codexBusy.value = true
  try {
    codexRaw.value = await store.generateCodexConfigToml()
    message.success(t('settings.toast.generatedToml'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    codexBusy.value = false
  }
}

async function saveCodexRaw() {
  codexBusy.value = true
  try {
    const path = await store.writeCodexConfigTomlRaw(codexRaw.value)
    message.success(t('settings.toast.saved', { path: path || codexPath.value }))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    codexBusy.value = false
  }
}

async function mergeWriteCodex() {
  codexBusy.value = true
  try {
    const path = await store.writeCodexConfigToml()
    message.success(t('settings.toast.mergedWritten', { path: path || codexPath.value }))
    await loadCodexRaw()
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    codexBusy.value = false
  }
}

async function restoreCodex() {
  dialog.warning({
    title: t('settings.dialog.restoreCodex.title'),
    content: t('settings.dialog.restoreCodex.content'),
    positiveText: t('settings.dialog.restoreCodex.ok'),
    negativeText: t('settings.dialog.restoreCodex.cancel'),
    onPositiveClick: async () => {
      codexBusy.value = true
      try {
        const path = await store.restoreCodexConfigToml()
        message.success(t('settings.toast.restored', { path: path || codexPath.value }))
        await loadCodexRaw()
      } catch (error) {
        message.error(error instanceof Error ? error.message : String(error))
      } finally {
        codexBusy.value = false
      }
    },
  })
}

async function restoreSelectedBackup() {
  if (!selectedBackup.value) {
    message.warning(t('settings.toast.needSelectBackup'))
    return
  }
  codexBusy.value = true
  try {
    const path = await store.restoreCodexConfigTomlFromBackup(selectedBackup.value)
    message.success(t('settings.toast.restored', { path: path || codexPath.value }))
    await loadCodexRaw()
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    codexBusy.value = false
  }
}

async function deleteSelectedBackup() {
  if (!selectedBackup.value) {
    message.warning(t('settings.toast.needSelectBackup'))
    return
  }
  dialog.warning({
    title: t('settings.dialog.deleteBackup.title'),
    content: t('settings.dialog.deleteBackup.content', { name: selectedBackup.value.split('/').slice(-1)[0] }),
    positiveText: t('settings.dialog.deleteBackup.ok'),
    negativeText: t('settings.dialog.deleteBackup.cancel'),
    onPositiveClick: async () => {
      codexBusy.value = true
      try {
        await store.deleteCodexConfigBackup(selectedBackup.value)
        selectedBackup.value = ''
        await refreshCodexBackups()
        message.success(t('settings.toast.deletedBackup'))
      } catch (error) {
        message.error(error instanceof Error ? error.message : String(error))
      } finally {
        codexBusy.value = false
      }
    },
  })
}

async function clearAllBackups() {
  dialog.warning({
    title: t('settings.dialog.clearBackups.title'),
    content: t('settings.dialog.clearBackups.content'),
    positiveText: t('settings.dialog.clearBackups.ok'),
    negativeText: t('settings.dialog.clearBackups.cancel'),
    onPositiveClick: async () => {
      codexBusy.value = true
      try {
        const removed = await store.clearCodexConfigBackups()
        selectedBackup.value = ''
        await refreshCodexBackups()
        message.success(t('settings.toast.clearedBackups', { count: removed }))
      } catch (error) {
        message.error(error instanceof Error ? error.message : String(error))
      } finally {
        codexBusy.value = false
      }
    },
  })
}
</script>

<template>
  <n-drawer
    :show="modelValue"
    placement="right"
    :width="520"
    @update:show="(value: boolean) => emit('update:modelValue', value)"
  >
    <n-drawer-content :title="t('settings.title')" closable>
      <div class="drawer-body">
        <n-card size="small" embedded>
          <n-space vertical size="large">
            <n-switch v-model:value="localConfig.enableAutoStart">
              <template #checked>{{ t('settings.switches.autoStart') }}</template>
              <template #unchecked>{{ t('settings.switches.autoStart') }}</template>
            </n-switch>
            <n-switch v-model:value="localConfig.minimizeToTray">
              <template #checked>{{ t('settings.switches.minimizeToTray') }}</template>
              <template #unchecked>{{ t('settings.switches.minimizeToTray') }}</template>
            </n-switch>
            <n-switch v-model:value="localConfig.compactMode">
              <template #checked>{{ t('settings.switches.compactMode') }}</template>
              <template #unchecked>{{ t('settings.switches.compactMode') }}</template>
            </n-switch>
          </n-space>
        </n-card>

        <n-form label-placement="top">
          <n-form-item :label="t('settings.form.logRetentionDays')">
            <n-input-number v-model:value="localConfig.logRetentionDays" :min="1" :max="30" />
          </n-form-item>
        </n-form>

        <n-space>
          <n-button type="primary" @click="submit">{{ t('settings.actions.save') }}</n-button>
          <n-button secondary @click="emit('export')">{{ t('settings.actions.exportConfig') }}</n-button>
        </n-space>

        <n-card size="small" embedded>
          <n-space vertical size="small">
            <div>
              <n-text style="font-weight: 600">{{ t('settings.codex.title') }}</n-text>
              <n-text depth="3" style="display: block; margin-top: 6px; line-height: 1.6">
                {{ t('settings.codex.desc') }}
              </n-text>
            </div>
            <n-space>
              <n-button secondary @click="emit('codexCopy')">{{ t('settings.actions.copyToml') }}</n-button>
              <n-button type="primary" @click="emit('codexWrite')">{{ t('settings.actions.writeFile') }}</n-button>
              <n-button secondary @click="emit('codexWriteProfiles')">{{ t('settings.actions.writeFileProfiles') }}</n-button>
            </n-space>
            <n-form label-placement="top">
              <n-form-item :label="t('settings.codex.filePath')">
                <n-input :value="codexPath" readonly />
              </n-form-item>
              <n-form-item :label="t('settings.codex.content')">
                <n-input
                  v-model:value="codexRaw"
                  type="textarea"
                  :autosize="{ minRows: 10, maxRows: 22 }"
                  :disabled="codexBusy"
                />
              </n-form-item>
            </n-form>

            <div v-if="needsWireApiFix" class="warning-text">
              {{ t('settings.codex.wireApiFix') }}
            </div>

            <n-form label-placement="top">
              <n-form-item :label="t('settings.codex.backups')">
                <n-select
                  v-model:value="selectedBackup"
                  :options="backupOptions"
                  :placeholder="t('settings.codex.backupPlaceholder')"
                  :disabled="codexBusy"
                  filterable
                />
              </n-form-item>
            </n-form>

            <n-space>
              <n-button tertiary :loading="codexBusy" @click="loadCodexRaw">{{ t('settings.codexActions.readFile') }}</n-button>
              <n-button tertiary :loading="codexBusy" @click="generateCodexRaw">{{ t('settings.codexActions.generateTemplate') }}</n-button>
              <n-button secondary :loading="codexBusy" @click="saveCodexRaw">{{ t('settings.codexActions.saveOverwrite') }}</n-button>
              <n-button type="primary" :loading="codexBusy" @click="mergeWriteCodex">{{ t('settings.codexActions.mergeWrite') }}</n-button>
              <n-button tertiary :loading="codexBusy" @click="refreshCodexBackups">{{ t('settings.codexActions.refreshBackups') }}</n-button>
              <n-button secondary :loading="codexBusy" @click="restoreSelectedBackup">{{ t('settings.codexActions.restoreSelected') }}</n-button>
              <n-button tertiary :loading="codexBusy" @click="deleteSelectedBackup">{{ t('settings.codexActions.deleteSelected') }}</n-button>
              <n-button tertiary :loading="codexBusy" @click="clearAllBackups">{{ t('settings.codexActions.clearBackups') }}</n-button>
              <n-button tertiary :loading="codexBusy" @click="restoreCodex">{{ t('settings.codexActions.restoreLatest') }}</n-button>
            </n-space>
          </n-space>
        </n-card>
      </div>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.drawer-body {
  display: grid;
  gap: 20px;
}

.warning-text {
  font-size: 12px;
  line-height: 1.6;
  color: var(--warning);
}
</style>
