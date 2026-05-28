<script setup lang="ts">
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { getProviderPreset } from '../utils/providers'
import type { Profile } from '../types'

defineProps<{
  profiles: Profile[]
  currentProfileId: string
  loading: boolean
}>()

const emit = defineEmits<{
  switch: [id: string]
  edit: [id: string]
  delete: [id: string]
  monitor: [id: string]
}>()

const store = useAppStore()
const message = useMessage()
const { t } = useI18n()

function handleSwitch(id: string) {
  if (id === store.config.currentProfileId) return
  emit('switch', id)
}

function handleEdit(id: string) {
  emit('edit', id)
}

function handleDelete(id: string) {
  if (store.profileList.length < 2) {
    message.warning(t('profile.cannotDeleteLast'))
    return
  }
  emit('delete', id)
}

function handleMonitor(id: string) {
  emit('monitor', id)
}
</script>

<template>
  <div class="profile-list">
    <div
      v-for="profile in profiles"
      :key="profile.id"
      class="profile-item"
      :class="{ active: profile.id === currentProfileId }"
      @click="handleSwitch(profile.id)"
    >
      <div class="profile-item-info">
        <span class="profile-item-name">{{ profile.name }}</span>
        <span class="profile-item-provider">{{ getProviderPreset(profile.provider)?.label ?? profile.provider }}</span>
        <span class="profile-item-meta">{{ profile.baseURL }}</span>
        <span v-if="profile.defaultModel" class="profile-item-meta">{{ profile.defaultModel }}</span>
      </div>
      <div class="profile-item-actions" @click.stop>
        <n-button size="tiny" quaternary @click="handleEdit(profile.id)">
          {{ t('guide.step.one.edit') }}
        </n-button>
        <n-button size="tiny" quaternary @click="handleMonitor(profile.id)">
          {{ t('guide.step.one.monitor') }}
        </n-button>
        <n-button
          size="tiny"
          quaternary
          type="error"
          @click="handleDelete(profile.id)"
        >
          {{ t('common.delete') }}
        </n-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.profile-list {
  display: grid;
  gap: 6px;
}

.profile-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 10px;
  border: 1px solid var(--border);
  background: rgba(255, 255, 255, 0.5);
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s;
}

.profile-item:hover {
  background: rgba(22, 119, 255, 0.04);
  border-color: rgba(22, 119, 255, 0.18);
}

.profile-item.active {
  border-color: rgba(22, 119, 255, 0.35);
  background: rgba(22, 119, 255, 0.06);
}

.profile-item-info {
  display: grid;
  gap: 2px;
  min-width: 0;
  flex: 1;
}

.profile-item-name {
  font-size: 13px;
  font-weight: 600;
  color: rgba(11, 18, 32, 0.9);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.profile-item-provider {
  font-size: 11px;
  color: var(--muted);
}

.profile-item-meta {
  font-size: 10px;
  color: var(--muted);
  opacity: 0.7;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  word-break: break-all;
}

.profile-item-actions {
  display: flex;
  gap: 2px;
  flex-shrink: 0;
}
</style>
