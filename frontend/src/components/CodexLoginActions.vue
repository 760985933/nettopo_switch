<script setup lang="ts">
import { ref } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { useCodexStore } from '../stores/codex'

const props = defineProps<{
  profileId: string
}>()

const emit = defineEmits<{
  completed: [id: string]
}>()

const store = useAppStore()
const codexStore = useCodexStore()
const message = useMessage()
const { t } = useI18n()

const activeAction = ref<'plugin' | 'noaccount' | null>(null)
const loadingId = ref<string | null>(null)
const completedLogins = ref<Set<string>>(new Set())

function isActive(id: string) {
  return loadingId.value === id
}

function isCompleted(id: string) {
  return store.config.profiles[id]
    ? completedLogins.value.has(id)
    : false
}

async function handlePluginLogin() {
  const profile = store.config.profiles[props.profileId]
  if (!profile?.apiKey) {
    message.warning(t('guide.monitor.noKey'))
    return
  }
  loadingId.value = props.profileId
  activeAction.value = 'plugin'
  try {
    if (props.profileId !== store.config.currentProfileId) {
      await store.setCurrentProfile(props.profileId)
    }
    if (!store.isRunning) await store.startProxy()
    const path = await codexStore.pluginUnlockLogin()
    if (loadingId.value !== props.profileId || activeAction.value !== 'plugin') return
    completedLogins.value.add(props.profileId)
    emit('completed', props.profileId)
    message.success(t('app.toast.codexTomlWritten', { path }))
  } catch (error) {
    if (loadingId.value === props.profileId && activeAction.value === 'plugin') {
      message.error(error instanceof Error ? error.message : String(error))
    }
  } finally {
    if (loadingId.value === props.profileId && activeAction.value === 'plugin') {
      loadingId.value = null
      activeAction.value = null
    }
  }
}

async function handleNoAccountLogin() {
  const profile = store.config.profiles[props.profileId]
  if (!profile?.apiKey) {
    message.warning(t('guide.monitor.noKey'))
    return
  }
  loadingId.value = props.profileId
  activeAction.value = 'noaccount'
  try {
    if (props.profileId !== store.config.currentProfileId) {
      await store.setCurrentProfile(props.profileId)
    }
    if (!store.isRunning) await store.startProxy()
    const path = await codexStore.writeCodexConfigTomlProfiles()
    if (loadingId.value !== props.profileId || activeAction.value !== 'noaccount') return
    completedLogins.value.add(props.profileId)
    emit('completed', props.profileId)
    message.success(t('app.toast.codexTomlWritten', { path }))
  } catch (error) {
    if (loadingId.value === props.profileId && activeAction.value === 'noaccount') {
      message.error(error instanceof Error ? error.message : String(error))
    }
  } finally {
    if (loadingId.value === props.profileId && activeAction.value === 'noaccount') {
      loadingId.value = null
      activeAction.value = null
    }
  }
}

function handleStop() {
  loadingId.value = null
  activeAction.value = null
  message.info(t('guide.actions.stopped'))
}
</script>

<template>
  <template v-if="store.config.profiles[profileId]">
    <n-button
      size="small"
      :type="isActive(profileId) && activeAction === 'plugin' ? 'error' : (isCompleted(profileId) ? 'success' : (store.config.profiles[profileId].apiKey ? 'primary' : undefined))"
      :disabled="!store.config.profiles[profileId].apiKey || (isCompleted(profileId) && !(isActive(profileId) && activeAction === 'plugin'))"
      :title="store.config.profiles[profileId].apiKey ? t('guide.actions.pluginUnlockLoginTooltip') : t('guide.monitor.noKey')"
      @click.stop="isActive(profileId) && activeAction === 'plugin' ? handleStop() : handlePluginLogin()"
    >
      {{ isActive(profileId) && activeAction === 'plugin' ? t('guide.actions.stop') : (isCompleted(profileId) ? t('guide.actions.completed') : t('guide.actions.pluginUnlockLogin')) }}
    </n-button>
    <n-button
      size="small"
      :type="isActive(profileId) && activeAction === 'noaccount' ? 'error' : (isCompleted(profileId) ? 'success' : (store.config.profiles[profileId].apiKey ? 'primary' : undefined))"
      :disabled="!store.config.profiles[profileId].apiKey || (isCompleted(profileId) && !(isActive(profileId) && activeAction === 'noaccount'))"
      @click.stop="isActive(profileId) && activeAction === 'noaccount' ? handleStop() : handleNoAccountLogin()"
    >
      {{ isActive(profileId) && activeAction === 'noaccount' ? t('guide.actions.stop') : (isCompleted(profileId) ? t('guide.actions.completed') : t('guide.actions.noAccountLogin')) }}
    </n-button>
    <span class="actions-sep">|</span>
  </template>
</template>

<style scoped>
.actions-sep {
  color: var(--border);
  font-size: 12px;
  margin: 0 2px;
}
</style>
