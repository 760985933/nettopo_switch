<script setup lang="ts">
import { ref, computed } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { useCodexStore } from '../stores/codex'

const props = defineProps<{
  profileId: string
}>()

const store = useAppStore()
const codexStore = useCodexStore()
const message = useMessage()
const { t } = useI18n()

const busyAction = ref<'plugin' | 'noaccount' | null>(null)

const profile = computed(() => store.config.profiles[props.profileId])
const isThisProfileRunning = computed(() => store.isRunning && store.config.currentProfileId === props.profileId)

const pluginBtnType = computed(() => {
  if (busyAction.value === 'plugin' || isThisProfileRunning.value) return 'error' as const
  return profile.value?.apiKey ? ('primary' as const) : (undefined as any)
})

const pluginBtnLabel = computed(() => {
  if (busyAction.value === 'plugin' || isThisProfileRunning.value) return t('guide.actions.stop')
  return t('guide.actions.pluginUnlockLogin')
})

const noAccountBtnType = computed(() => {
  if (busyAction.value === 'noaccount' || isThisProfileRunning.value) return 'error' as const
  return profile.value?.apiKey ? ('primary' as const) : (undefined as any)
})

const noAccountBtnLabel = computed(() => {
  if (busyAction.value === 'noaccount' || isThisProfileRunning.value) return t('guide.actions.stop')
  return t('guide.actions.noAccountLogin')
})

const disabled = computed(() => !profile.value?.apiKey)

function onPluginClick() {
  if (busyAction.value === 'plugin') {
    busyAction.value = null
    message.info(t('guide.actions.stopped'))
  } else if (isThisProfileRunning.value) {
    store.stopProxy()
  } else {
    handlePluginLogin()
  }
}

function onNoAccountClick() {
  if (busyAction.value === 'noaccount') {
    busyAction.value = null
    message.info(t('guide.actions.stopped'))
  } else if (isThisProfileRunning.value) {
    store.stopProxy()
  } else {
    handleNoAccountLogin()
  }
}

async function handlePluginLogin() {
  if (!profile.value?.apiKey) {
    message.warning(t('guide.monitor.noKey'))
    return
  }
  busyAction.value = 'plugin'
  try {
    if (props.profileId !== store.config.currentProfileId) {
      await store.setCurrentProfile(props.profileId)
    }
    if (!store.isRunning) await store.startProxy()
    const path = await codexStore.pluginUnlockLogin()
    if (busyAction.value !== 'plugin') return
    message.success(t('app.toast.codexTomlWritten', { path }))
  } catch (error) {
    if (busyAction.value === 'plugin') {
      message.error(error instanceof Error ? error.message : String(error))
    }
  } finally {
    if (busyAction.value === 'plugin') {
      busyAction.value = null
    }
  }
}

async function handleNoAccountLogin() {
  if (!profile.value?.apiKey) {
    message.warning(t('guide.monitor.noKey'))
    return
  }
  busyAction.value = 'noaccount'
  try {
    if (props.profileId !== store.config.currentProfileId) {
      await store.setCurrentProfile(props.profileId)
    }
    if (!store.isRunning) await store.startProxy()
    const path = await codexStore.writeCodexConfigTomlProfiles()
    if (busyAction.value !== 'noaccount') return
    message.success(t('app.toast.codexTomlWritten', { path }))
  } catch (error) {
    if (busyAction.value === 'noaccount') {
      message.error(error instanceof Error ? error.message : String(error))
    }
  } finally {
    if (busyAction.value === 'noaccount') {
      busyAction.value = null
    }
  }
}
</script>

<template>
  <n-button
    v-if="profile"
    size="small"
    :type="pluginBtnType"
    :disabled="disabled"
    @click.stop="onPluginClick"
  >
    {{ pluginBtnLabel }}
  </n-button>
  <n-button
    v-if="profile"
    size="small"
    :type="noAccountBtnType"
    :disabled="disabled"
    @click.stop="onNoAccountClick"
  >
    {{ noAccountBtnLabel }}
  </n-button>
  <span v-if="profile" class="actions-sep">|</span>
</template>

<style scoped>
.actions-sep {
  color: var(--border);
  font-size: 12px;
  margin: 0 2px;
}
</style>
