<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { ClipboardSetText } from '../../wailsjs/runtime/runtime'
import type { AppConfig } from '../types'
import { useAppStore } from '../stores/app'
import { useCodexStore } from '../stores/codex'
import { useUiStore } from '../stores/ui'

const props = defineProps<{
  modelValue: boolean
  config: AppConfig
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const localConfig = ref<AppConfig>({ ...props.config })
const store = useAppStore()
const codexStore = useCodexStore()
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

async function submit() {
  try {
    await store.saveConfig(localConfig.value)
    emit('update:modelValue', false)
    message.success(t('app.toast.settingsSaved'))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  }
}

async function handleExport() {
  try {
    const content = await store.exportConfig()
    await ClipboardSetText(content)
    message.success(t('app.toast.configJsonCopied'))
  } catch (error) {
    dialog.warning({
      title: t('app.dialog.exportConfig.title'),
      content: error instanceof Error ? error.message : String(error),
      positiveText: t('app.dialog.exportConfig.ok'),
    })
  }
}

async function handleCodexCopy() {
  try {
    const content = await codexStore.generateCodexConfigToml()
    await ClipboardSetText(content)
    message.success(t('app.toast.codexTomlCopied'))
  } catch (error) {
    dialog.warning({
      title: t('app.dialog.codexCopy.title'),
      content: error instanceof Error ? error.message : String(error),
      positiveText: t('app.dialog.codexCopy.ok'),
    })
  }
}

async function handleCodexWrite() {
  try {
    const path = await codexStore.writeCodexConfigToml()
    const hintPath = await codexStore.getCodexConfigPath()
    message.success(t('app.toast.codexTomlWritten', { path: path || hintPath }))
  } catch (error) {
    dialog.warning({
      title: t('app.dialog.codexWrite.title'),
      content: error instanceof Error ? error.message : String(error),
      positiveText: t('app.dialog.codexWrite.ok'),
    })
  }
}

async function handleCodexWriteProfiles() {
  try {
    const path = await codexStore.writeCodexConfigTomlProfiles()
    const hintPath = await codexStore.getCodexConfigPath()
    message.success(t('app.toast.codexTomlWritten', { path: path || hintPath }))
  } catch (error) {
    dialog.warning({
      title: t('app.dialog.codexWrite.title'),
      content: error instanceof Error ? error.message : String(error),
      positiveText: t('app.dialog.codexWrite.ok'),
    })
  }
}

async function loadCodexRaw() {
  codexBusy.value = true
  try {
    codexPath.value = await codexStore.getCodexConfigPath()
    codexRaw.value = await codexStore.readCodexConfigToml()
    await refreshCodexBackups()
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    codexBusy.value = false
  }
}

async function refreshCodexBackups() {
  codexBackups.value = await codexStore.listCodexConfigBackups()
  if (selectedBackup.value && !codexBackups.value.includes(selectedBackup.value)) {
    selectedBackup.value = ''
  }
}

async function generateCodexRaw() {
  codexBusy.value = true
  try {
    codexRaw.value = await codexStore.generateCodexConfigToml()
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
    const path = await codexStore.writeCodexConfigTomlRaw(codexRaw.value)
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
    const path = await codexStore.writeCodexConfigToml()
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
        const path = await codexStore.restoreCodexConfigToml()
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
    const path = await codexStore.restoreCodexConfigTomlFromBackup(selectedBackup.value)
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
        await codexStore.deleteCodexConfigBackup(selectedBackup.value)
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
        const removed = await codexStore.clearCodexConfigBackups()
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
        <n-alert type="info" :bordered="false" closable>
          <template #header>{{ t('settings.behaviorMoved') }}</template>
          {{ t('settings.behaviorMovedDesc') }}
        </n-alert>

        <n-space class="settings-actions">
          <n-button secondary size="small" @click="handleExport">{{ t('settings.actions.exportConfig') }}</n-button>
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
              <n-button secondary @click="handleCodexCopy">{{ t('settings.actions.copyToml') }}</n-button>
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

.settings-group {
  display: grid;
  gap: 16px;
  padding: 16px;
  border: 1px dashed var(--border);
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.4);
}

.settings-group-label {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--muted);
}

.switches-row {
  display: grid;
  gap: 10px;
}

.switch-item {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  color: rgba(11, 18, 32, 0.86);
}

.log-days-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.log-days-label {
  font-size: 13px;
  color: rgba(11, 18, 32, 0.86);
}

.settings-actions {
  padding-top: 4px;
}

.warning-text {
  font-size: 12px;
  line-height: 1.6;
  color: var(--warning);
}
</style>
