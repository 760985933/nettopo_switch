<script setup lang="ts">
import { ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'

const store = useAppStore()
const message = useMessage()
const { t } = useI18n()

const localConfig = ref({ ...store.config })

watch(
  () => store.config,
  (value) => {
    localConfig.value = { ...value }
  },
  { deep: true, immediate: true },
)

async function handleSave() {
  await store.saveConfig(localConfig.value)
  message.success(t('app.toast.settingsSaved'))
}
</script>

<template>
  <div class="settings-page">
    <div class="page-head">
      <h2>{{ t('globalConfig.title') }}</h2>
    </div>

    <div class="card">
      <div class="card-header">
        <span class="card-title">{{ t('globalConfig.behavior') }}</span>
      </div>
      <div class="settings-grid">
        <div class="settings-item">
          <span class="settings-label">{{ t('proxy.minimizeToTray') }}</span>
          <n-switch v-model:value="localConfig.minimizeToTray" />
        </div>
      </div>
    </div>

    <div class="action-bar">
      <n-button type="primary" :loading="store.isBusy" @click="handleSave">
        {{ t('globalConfig.save') }}
      </n-button>
    </div>
  </div>
</template>

<style scoped>
.settings-page {
  display: grid;
  gap: 14px;
}

.page-head {
  padding: 18px;
  border-radius: 22px;
  border: 1px solid var(--border);
  background: var(--surface);
  box-shadow: 0 10px 30px rgba(14, 30, 68, 0.08);
}

.page-head h2 {
  margin: 0;
  font-size: 18px;
  color: var(--text);
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

.settings-grid {
  display: grid;
  gap: 4px;
}

.settings-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 0;
}

.settings-label {
  font-size: 13px;
  color: rgba(11, 18, 32, 0.86);
}

.action-bar {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
