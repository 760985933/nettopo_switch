<script setup lang="ts">
import { reactive, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { GetUsageBalance } from '../../wailsjs/go/main/App'
import type { Profile, UsageBalance } from '../types'

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
  stop: []
  pluginLogin: [id: string]
  noaccountLogin: [id: string]
}>()

const store = useAppStore()
const message = useMessage()
const { t } = useI18n()

const usageData = reactive<Record<string, UsageBalance | null>>({})
const usageLoadingMap = reactive<Record<string, boolean>>({})

async function fetchUsage(id: string) {
  const profile = props.profiles.find(p => p.id === id)
  if (!profile?.apiKey) return
  usageLoadingMap[id] = true
  try {
    usageData[id] = await GetUsageBalance(id)
  } catch (err) {
    usageData[id] = { availableBalance: '', totalBalance: '', currency: '', isDepleted: false, error: String(err) }
  } finally {
    usageLoadingMap[id] = false
  }
}

watch(() => props.profiles, (profiles) => {
  profiles.forEach(p => {
    if (p.apiKey && !usageData[p.id]) fetchUsage(p.id)
  })
}, { immediate: true })

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

function isLoginDisabled(id: string, action: 'plugin' | 'noaccount') {
  if (props.loginProfileId === null) return false
  if (props.loginProfileId !== id) return true
  return props.activeLoginAction !== null && props.activeLoginAction !== action
}

function hasApiKey(id: string) {
  const profile = props.profiles.find(p => p.id === id)
  return !!profile?.apiKey
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
          </div>
          <span v-if="profile.baseURL" class="profile-item-meta">
            <span class="profile-item-label">API:</span> {{ profile.baseURL }}
          </span>
          <span v-if="profile.defaultModel" class="profile-item-meta">
            <span class="profile-item-label">{{ t('config.fields.defaultModel') }}:</span> {{ profile.defaultModel }}
          </span>
        </div>
        <div class="profile-item-right">
          <div v-if="usageData[profile.id]" class="profile-item-usage" @click.stop>
            <template v-if="usageData[profile.id]?.error">
              <span class="usage-error">{{ usageData[profile.id]?.error }}</span>
            </template>
            <template v-else>
              <span>{{ t('guide.usage.available') }}: {{ usageData[profile.id]?.availableBalance }} {{ usageData[profile.id]?.currency }}</span>
              <span class="usage-sep">/</span>
              <span>{{ t('guide.usage.total') }}: {{ usageData[profile.id]?.totalBalance }} {{ usageData[profile.id]?.currency }}</span>
              <span v-if="usageData[profile.id]?.isDepleted" class="usage-depleted">{{ t('guide.usage.depleted') }}</span>
            </template>
            <n-button text size="tiny" :loading="usageLoadingMap[profile.id]" @click="fetchUsage(profile.id)">
              <template #icon>
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"/></svg>
              </template>
            </n-button>
          </div>
          <div class="profile-item-actions" @click.stop>
          <template v-if="proxyRunning && profile.id === currentProfileId">
            <n-button
              size="small"
              type="error"
              :loading="loading"
              @click="emit('stop')"
            >
              <template #icon>
                <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="6" width="12" height="12" rx="2"/></svg>
              </template>
              {{ t('config.actions.stop') }}
            </n-button>
          </template>
          <template v-else>
            <n-button
              size="small"
              type="primary"
              :title="t('guide.actions.pluginUnlockLoginTooltip')"
              :disabled="isLoginDisabled(profile.id, 'noaccount') || !hasApiKey(profile.id)"
              :loading="loginProfileId === profile.id && activeLoginAction === 'plugin'"
              @click="emit('pluginLogin', profile.id)"
            >
              {{ t('guide.actions.pluginUnlockLogin') }}
            </n-button>
            <n-button
              size="small"
              secondary
              type="primary"
              :disabled="isLoginDisabled(profile.id, 'plugin') || !hasApiKey(profile.id)"
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
          <n-button size="small" tertiary type="error" @click="handleDelete(profile.id)">
            {{ t('common.delete') }}
          </n-button>
        </div>
      </div>
      <div v-if="usageData[profile.id]" class="profile-item-usage" @click.stop>
        <template v-if="usageData[profile.id]?.error">
          <span class="usage-error">{{ usageData[profile.id]?.error }}</span>
        </template>
        <template v-else>
          <span>{{ t('guide.usage.available') }}: {{ usageData[profile.id]?.availableBalance }} {{ usageData[profile.id]?.currency }}</span>
          <span class="usage-sep">/</span>
          <span>{{ t('guide.usage.total') }}: {{ usageData[profile.id]?.totalBalance }} {{ usageData[profile.id]?.currency }}</span>
          <span v-if="usageData[profile.id]?.isDepleted" class="usage-depleted">{{ t('guide.usage.depleted') }}</span>
        </template>
        <n-button text size="tiny" :loading="usageLoadingMap[profile.id]" @click="fetchUsage(profile.id)">
          <template #icon>
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"/></svg>
          </template>
        </n-button>
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
  font-size: 11px;
  color: rgba(11, 18, 32, 0.55);
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
  font-size: 11px;
  color: rgba(11, 18, 32, 0.6);
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

.profile-item-usage {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: rgba(19, 160, 90, 0.88);
  font-weight: 500;
  padding-top: 6px;
  border-top: 1px dashed var(--border);
  margin-top: 2px;
  flex-wrap: wrap;
}

.usage-sep {
  color: var(--border);
}

.usage-depleted {
  color: rgba(212, 56, 13, 0.92);
  font-weight: 600;
}

.usage-error {
  color: var(--muted);
  font-size: 10px;
  word-break: break-word;
}
</style>
