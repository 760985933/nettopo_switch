<script setup lang="ts">
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { getProviderPreset } from '../utils/providers'
import type { Profile } from '../types'

const props = defineProps<{
  profiles: Profile[]
  currentProfileId: string
  loading: boolean
  proxyRunning: boolean
  loginProfileId: string | null
  activeLoginAction: 'plugin' | 'noaccount' | null
}>()

const emit = defineEmits<{
  edit: [id: string]
  delete: [id: string]
  monitor: [id: string]
  stop: []
  pluginLogin: [id: string]
  noaccountLogin: [id: string]
}>()

const store = useAppStore()
const message = useMessage()
const { t } = useI18n()

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

function isLoginDisabled(id: string, action: 'plugin' | 'noaccount') {
  // Disabled when another profile is logging in, or same profile doing other action
  if (props.loginProfileId === null) return false
  if (props.loginProfileId !== id) return true
  return props.activeLoginAction !== null && props.activeLoginAction !== action
}
</script>

<template>
  <div class="profile-list">
    <div
      v-for="profile in profiles"
      :key="profile.id"
      class="profile-item"
      :class="{ active: profile.id === currentProfileId }"
    >
      <div class="profile-item-main">
        <div class="profile-item-info">
          <div class="profile-item-name-row">
            <span v-if="profile.name" class="profile-item-label">{{ t('config.fields.profileName') }}:</span>
            <span class="profile-item-name">{{ profile.name }}</span>
            <span class="profile-item-provider">{{ getProviderPreset(profile.provider)?.label ?? profile.provider }}</span>
          </div>
          <span v-if="profile.baseURL" class="profile-item-meta">
            <span class="profile-item-label">API:</span> {{ profile.baseURL }}
          </span>
          <span v-if="profile.defaultModel" class="profile-item-meta">
            <span class="profile-item-label">{{ t('config.fields.defaultModel') }}:</span> {{ profile.defaultModel }}
          </span>
        </div>
        <div class="profile-item-actions" @click.stop>
          <template v-if="proxyRunning && profile.id === currentProfileId">
            <n-button
              size="small"
              tertiary
              type="error"
              :loading="loading"
              @click="emit('stop')"
            >
              {{ t('config.actions.stop') }}
            </n-button>
          </template>
          <template v-else>
            <n-button
              size="small"
              type="primary"
              :disabled="isLoginDisabled(profile.id, 'noaccount')"
              :loading="loginProfileId === profile.id && activeLoginAction === 'plugin'"
              @click="emit('pluginLogin', profile.id)"
            >
              {{ t('guide.actions.pluginUnlockLogin') }}
            </n-button>
            <n-button
              size="small"
              secondary
              type="primary"
              :disabled="isLoginDisabled(profile.id, 'plugin')"
              :loading="loginProfileId === profile.id && activeLoginAction === 'noaccount'"
              @click="emit('noaccountLogin', profile.id)"
            >
              {{ t('guide.actions.noAccountLogin') }}
            </n-button>
          </template>
          <div class="profile-item-actions-sep" />
          <n-button size="small" tertiary @click="handleEdit(profile.id)">
            {{ t('guide.step.one.edit') }}
          </n-button>
          <n-button size="small" tertiary @click="handleMonitor(profile.id)">
            {{ t('guide.step.one.monitor') }}
          </n-button>
          <n-button size="small" tertiary type="error" @click="handleDelete(profile.id)">
            {{ t('common.delete') }}
          </n-button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.profile-list {
  display: grid;
  gap: 8px;
}

.profile-item {
  padding: 10px 12px;
  border-radius: 12px;
  border: 1px solid var(--border);
  background: rgba(255, 255, 255, 0.5);
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s, box-shadow 0.15s;
}

.profile-item:hover {
  background: rgba(255, 255, 255, 0.8);
  border-color: rgba(22, 119, 255, 0.18);
  box-shadow: 0 2px 8px rgba(14, 30, 68, 0.06);
}

.profile-item.active {
  border-color: rgba(22, 119, 255, 0.35);
  background: rgba(22, 119, 255, 0.06);
}

.profile-item-main {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.profile-item-info {
  display: grid;
  gap: 2px;
  min-width: 0;
  flex: 1;
}

.profile-item-name-row {
  display: flex;
  align-items: baseline;
  gap: 6px;
  flex-wrap: wrap;
}

.profile-item-label {
  font-size: 10px;
  color: var(--muted);
  opacity: 0.7;
  white-space: nowrap;
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
  align-items: center;
  flex-wrap: wrap;
}

.profile-item-actions-sep {
  width: 1px;
  height: 14px;
  background: var(--border);
  margin: 0 4px;
}
</style>
