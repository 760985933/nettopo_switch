<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { useUiStore } from '../stores/ui'
import type { ProxyStatusPayload, HealthCheckResult } from '../types'
import { PROVIDER_PRESETS, getProviderPreset } from '../utils/providers'
import ConfigPanel from './ConfigPanel.vue'
import ProfileList from './ProfileList.vue'
import MonitorPanel from './MonitorPanel.vue'

const emit = defineEmits<{
  copy: [value: string]
  health: []
  stop: []
  refresh: []
}>()

const props = defineProps<{
  listenAddress: string
  loading: boolean
  status: ProxyStatusPayload
  health: HealthCheckResult | null
}>()

const store = useAppStore()
const ui = useUiStore()
const message = useMessage()
const { t } = useI18n()

const statusLabel = computed(() => {
  switch (props.status.status) {
    case 'running':
      return t('app.status.running')
    case 'starting':
      return t('app.status.starting')
    case 'error':
      return t('app.status.error')
    default:
      return t('app.status.stopped')
  }
})

const healthSummary = computed(() => {
  if (!props.health) return null
  const failed = props.health.checks.filter((item) => !item.ok)
  if (props.health.ok) return { tone: 'success' as const, text: t('console.health.ok') }
  return { tone: 'warning' as const, text: t('console.health.failed', { count: failed.length }) }
})

const failedChecks = computed(() => {
  if (!props.health) return []
  return props.health.checks.filter((item) => !item.ok)
})

const codexBaseURL = computed(() => {
  if (!props.listenAddress) return ''
  return props.listenAddress.replace(/\/+$/, '') + '/v1'
})

// ── Add profile dialog ──
const showAddDialog = ref(false)
const adding = ref(false)
const newProfileName = ref('')
const newProfileProvider = ref('deepseek')
const addProfileProviderOptions = PROVIDER_PRESETS.map((p) => ({
  label: p.label,
  value: p.id,
}))

function openAddDialog() {
  newProfileName.value = ''
  newProfileProvider.value = 'deepseek'
  showAddDialog.value = true
}

async function handleAddProfile() {
  const name = newProfileName.value.trim()
  if (!name) return
  adding.value = true
  try {
    await store.addProfile(name, newProfileProvider.value)
    newProfileName.value = ''
    showAddDialog.value = false
    message.success(t('profile.added', { name }))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    adding.value = false
  }
}

// ── Delete profile ──
const showDeleteConfirm = ref(false)
const deleting = ref(false)
const deletingProfileId = ref<string | null>(null)

function confirmDeleteProfile(id: string) {
  deletingProfileId.value = id
  showDeleteConfirm.value = true
}

async function handleDeleteProfile() {
  const id = deletingProfileId.value
  if (!id) return
  const profile = store.config.profiles[id]
  if (!profile) return
  deleting.value = true
  try {
    await store.deleteProfile(id)
    showDeleteConfirm.value = false
    deletingProfileId.value = null
    message.success(t('profile.deleted', { name: profile.name }))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    deleting.value = false
  }
}

// ── Login actions ──
const activeLoginAction = ref<'plugin' | 'noaccount' | null>(null)
const loginProfileId = ref<string | null>(null)

// ── Config drawer ──
const configDrawerVisible = ref(false)
const editingProfileId = ref<string | null>(null)

function handleEditProfile(id: string) {
  editingProfileId.value = id
  configDrawerVisible.value = true
}

function handleConfigSaved() {
  configDrawerVisible.value = false
  editingProfileId.value = null
}

// ── Monitor modal ──
const monitorVisible = ref(false)
const monitorRef = ref<InstanceType<typeof MonitorPanel> | null>(null)
const monitoringProfileId = ref<string | undefined>()

function openMonitor(id: string) {
  monitoringProfileId.value = id
  monitorVisible.value = true
  setTimeout(() => monitorRef.value?.open(), 0)
}

// ── Per-profile login ──
async function handleProfilePluginLogin(id: string) {
  loginProfileId.value = id
  activeLoginAction.value = 'plugin'
  try {
    if (id !== store.config.currentProfileId) {
      const wasRunning = store.isRunning
      await store.setCurrentProfile(id)
      if (wasRunning) await store.restartProxy()
    }
    if (!store.isRunning) await store.startProxy()
    const path = await store.pluginUnlockLogin()
    const hintPath = await store.getCodexConfigPath()
    message.success(t('app.toast.codexTomlWritten', { path: path || hintPath }))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    loginProfileId.value = null
    activeLoginAction.value = null
  }
}

async function handleProfileNoAccountLogin(id: string) {
  loginProfileId.value = id
  activeLoginAction.value = 'noaccount'
  try {
    if (id !== store.config.currentProfileId) {
      const wasRunning = store.isRunning
      await store.setCurrentProfile(id)
      if (wasRunning) await store.restartProxy()
    }
    if (!store.isRunning) await store.startProxy()
    const path = await store.writeCodexConfigTomlProfiles()
    const hintPath = await store.getCodexConfigPath()
    message.success(t('app.toast.codexTomlWritten', { path: path || hintPath }))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    loginProfileId.value = null
    activeLoginAction.value = null
  }
}

// ── Restore default ──
async function handleRestoreCodex() {
  try {
    const path = await store.restoreCodexConfigToml()
    message.success(t('settings.toast.restored', { path }))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  }
}

// ── Sandbox ──
const showSandbox = ref(false)
const networkAccess = ref(true)
const sandboxMode = ref('workspace-write')
const approvalPolicy = ref('on-request')
const sandboxConfigLoading = ref(false)
let sandboxConfigLoaded = false

async function loadSandboxConfig() {
  try {
    const cfg = await store.getSandboxConfig()
    networkAccess.value = cfg.networkAccess
    sandboxMode.value = cfg.sandboxMode || 'workspace-write'
    approvalPolicy.value = cfg.approvalPolicy || 'on-request'
    sandboxConfigLoaded = true
    await store.setSandboxConfig({
      networkAccess: networkAccess.value,
      sandboxMode: sandboxMode.value,
      approvalPolicy: approvalPolicy.value,
    })
  } catch (err) {
    console.error('load sandbox config failed', err)
  }
}

async function saveSandboxConfig() {
  sandboxConfigLoading.value = true
  try {
    await store.setSandboxConfig({
      networkAccess: networkAccess.value,
      sandboxMode: sandboxMode.value,
      approvalPolicy: approvalPolicy.value,
    })
    message.info(t('guide.sandbox.configHint'))
  } catch (err) {
    message.error(String(err))
  } finally {
    sandboxConfigLoading.value = false
  }
}

const sandboxModeOptions = [
  { label: t('guide.sandbox.sandboxModeOptions.readOnly'), value: 'read-only' },
  { label: t('guide.sandbox.sandboxModeOptions.workspaceWrite'), value: 'workspace-write' },
  { label: t('guide.sandbox.sandboxModeOptions.dangerFullAccess'), value: 'danger-full-access' },
]

const approvalPolicyOptions = [
  { label: t('guide.sandbox.approvalPolicyOptions.untrusted'), value: 'untrusted' },
  { label: t('guide.sandbox.approvalPolicyOptions.onRequest'), value: 'on-request' },
  { label: t('guide.sandbox.approvalPolicyOptions.never'), value: 'never' },
]

watch(showSandbox, (v) => {
  if (v) loadSandboxConfig()
})

</script>

<template>
  <div class="guide-card">
    <div class="guide-header">
      <div>
        <h3>{{ t('guide.title') }}</h3>
      </div>
      <div class="guide-header-actions">
        <n-button size="small" type="warning" secondary @click="openAddDialog">
          <template #icon>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
          </template>
          {{ t('profile.add') }}
        </n-button>
        <n-button
          tertiary
          type="primary"
          :disabled="!codexBaseURL"
          @click="emit('copy', codexBaseURL)"
        >
          {{ t('guide.actions.copyBaseUrl') }}
        </n-button>
      </div>
    </div>

    <div class="guide-body">
      <!-- Left: Step 1 -->
      <div class="guide-main">
        <div class="step">
          <div class="step-head">
            <span class="step-badge">Step 1</span>
            <span class="step-title">{{ t('guide.step.one.title') }}</span>
          </div>
          <div class="step-body">
            <ProfileList
              :profiles="store.profileList"
              :current-profile-id="store.config.currentProfileId"
              :loading="loading"
              :proxy-running="store.isRunning"
              :login-profile-id="loginProfileId"
              :active-login-action="activeLoginAction"
              @edit="handleEditProfile"
              @delete="confirmDeleteProfile"
              @monitor="openMonitor"
              @stop="emit('stop')"
              @plugin-login="handleProfilePluginLogin"
              @noaccount-login="handleProfileNoAccountLogin"
            />

          </div>
        </div>
      </div>

      <!-- Right: Step 2 + Step 3 -->
      <div class="guide-side">
        <div class="step">
          <div class="step-head">
            <span class="step-badge">{{ t('guide.step.two.title') }}</span>
          </div>
          <div class="step-body">
            <div class="actions">
              <n-button tertiary @click="ui.showSettings = true">{{ t('guide.actions.preferences') }}</n-button>
              <n-button tertiary @click="handleRestoreCodex">{{ t('guide.actions.restoreDefault') }}</n-button>
              <n-button tertiary @click="showSandbox = true">{{ t('guide.actions.sandbox') }}</n-button>
            </div>
            <div class="restart-hint">{{ t('guide.sandbox.configHint') }}</div>
          </div>
        </div>

        <div class="step">
          <div class="step-head">
            <span class="step-badge">{{ t('guide.step.three.title') }}</span>
          </div>
          <div class="step-body">
            <div class="s-status">
              <span class="s-dot" :data-status="status.status" />
              <span>{{ statusLabel }}</span>
            </div>

            <div class="s-meta">
              <span class="s-meta-item">
                <span class="s-meta-label">{{ t('console.meta.listenAddress') }}:</span>
                <strong>{{ status.listenAddress || t('console.meta.notRunning') }}</strong>
              </span>
              <span class="s-meta-item">
                <span class="s-meta-label">{{ t('console.meta.requestCount') }}:</span>
                <strong>{{ status.requestCount }}</strong>
              </span>
              <span v-if="status.lastError" class="s-meta-item" data-tone="error">
                <span class="s-meta-label">{{ t('console.meta.lastError') }}:</span>
                <strong>{{ status.lastError }}</strong>
              </span>
            </div>

            <div v-if="healthSummary" class="s-health" :data-tone="healthSummary.tone">
              <span class="h-dot" />
              <span>{{ healthSummary.text }}</span>
            </div>
            <div v-if="failedChecks.length" class="s-fails">
              <div v-for="item in failedChecks" :key="item.name" class="s-fail">
                <strong>{{ item.name }}</strong>
                <p>{{ item.message }}</p>
              </div>
            </div>

            <div class="actions">
              <n-button type="primary" :loading="loading" @click="emit('health')">{{ t('guide.step.three.healthCheck') }}</n-button>
              <n-button secondary :loading="loading" @click="emit('refresh')">{{ t('console.actions.refresh') }}</n-button>
            </div>

            <div class="hint">{{ t('guide.step.three.hint') }}</div>
            <div class="cmd">
              <div class="cmd-label">{{ t('guide.step.three.quickVerify') }}</div>
              <div class="mono">浏览器访问 {{ props.listenAddress || 'http://127.0.0.1:11434' }}/health</div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Add profile dialog -->
    <n-modal
      v-model:show="showAddDialog"
      :title="t('profile.addTitle')"
      preset="dialog"
      :positive-text="t('profile.confirmAdd')"
      :negative-text="t('profile.cancelAdd')"
      :loading="adding"
      @positive-click="handleAddProfile"
      @negative-click="showAddDialog = false"
    >
      <div style="display: grid; gap: 12px;">
        <n-input
          v-model:value="newProfileName"
          :placeholder="t('profile.namePlaceholder')"
          @keyup.enter="handleAddProfile"
        />
        <n-select
          v-model:value="newProfileProvider"
          :options="addProfileProviderOptions"
          :placeholder="t('profile.providerPlaceholder')"
        />
      </div>
    </n-modal>

    <!-- Delete profile confirmation -->
    <n-modal
      v-model:show="showDeleteConfirm"
      preset="dialog"
      :title="t('common.delete')"
      :content="t('profile.confirmDelete', { name: deletingProfileId ? (store.config.profiles[deletingProfileId]?.name ?? '') : '' })"
      :positive-text="t('common.delete')"
      :negative-text="t('profile.cancelAdd')"
      type="warning"
      :loading="deleting"
      @positive-click="handleDeleteProfile"
      @negative-click="showDeleteConfirm = false"
    />

    <!-- Config editor drawer -->
    <n-drawer
      v-model:show="configDrawerVisible"
      placement="right"
      :width="520"
      @update:show="(v: boolean) => { if (!v) editingProfileId = null }"
    >
      <n-drawer-content :title="t('config.title')" closable>
        <ConfigPanel
          v-if="configDrawerVisible"
          :profile-id="editingProfileId ?? undefined"
          @save="handleConfigSaved"
        />
      </n-drawer-content>
    </n-drawer>

    <!-- Monitor modal -->
    <n-modal
      v-model:show="monitorVisible"
      :title="t('guide.monitor.title')"
      preset="card"
      style="width: 480px; max-width: 90vw;"
    >
      <MonitorPanel ref="monitorRef" :profile-id="monitoringProfileId" />
    </n-modal>

    <!-- Sandbox modal -->
    <n-modal
      v-model:show="showSandbox"
      :title="t('guide.sandbox.title')"
      preset="card"
      style="width: 560px; max-width: 90vw;"
      :mask-closable="false"
    >
      <div class="sandbox">
        <div class="sandbox-fields">
          <div class="sandbox-field">
            <span class="sandbox-field-label">{{ t('guide.sandbox.networkAccess') }}</span>
            <n-switch
              v-model:value="networkAccess"
              :loading="sandboxConfigLoading"
              @update:value="saveSandboxConfig"
            />
          </div>
          <div class="sandbox-field">
            <span class="sandbox-field-label">{{ t('guide.sandbox.sandboxMode') }}</span>
            <n-select
              v-model:value="sandboxMode"
              :options="sandboxModeOptions"
              :loading="sandboxConfigLoading"
              size="small"
              class="sandbox-field-select"
              @update:value="saveSandboxConfig"
            />
          </div>
          <div class="sandbox-field">
            <span class="sandbox-field-label">{{ t('guide.sandbox.approvalPolicy') }}</span>
            <n-select
              v-model:value="approvalPolicy"
              :options="approvalPolicyOptions"
              :loading="sandboxConfigLoading"
              size="small"
              class="sandbox-field-select"
              @update:value="saveSandboxConfig"
            />
          </div>
        </div>
        <div class="restart-hint">{{ t('guide.sandbox.configHint') }}</div>
      </div>
    </n-modal>
  </div>
</template>

<style scoped>
.guide-card {
  display: grid;
  gap: 14px;
  padding: 18px;
  border-radius: 22px;
  border: 1px solid var(--border);
  background: var(--surface);
  box-shadow: 0 10px 30px rgba(14, 30, 68, 0.08);
}

.guide-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.guide-header-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.guide-header h3 {
  margin: 0;
  font-size: 16px;
  color: var(--text);
}

.guide-body {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: 14px;
  align-items: start;
}

.guide-main,
.guide-side {
  display: grid;
  gap: 10px;
}

.guide-side {
  position: sticky;
  top: 8px;
}

.steps {
  display: grid;
  gap: 10px;
}

.step {
  border-radius: 18px;
  border: 1px solid var(--border);
  background: rgba(255, 255, 255, 0.82);
  padding: 12px 12px 14px;
  display: grid;
  gap: 10px;
}

.step-head {
  display: flex;
  align-items: center;
  gap: 10px;
}

.step-head--clickable {
  cursor: pointer;
  user-select: none;
}

.step-head--clickable:hover .step-title {
  color: rgba(22, 119, 255, 0.85);
}

.step-chevron {
  margin-left: auto;
  font-size: 18px;
  line-height: 1;
  color: var(--muted);
  transition: transform 0.2s ease;
  transform: rotate(0deg);
}

.step-chevron.open {
  transform: rotate(90deg);
}

.step-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  background: rgba(22, 119, 255, 0.12);
  color: rgba(22, 119, 255, 0.92);
}

.step-title {
  font-size: 13px;
  font-weight: 600;
  color: rgba(11, 18, 32, 0.92);
}

.step-body {
  display: grid;
  gap: 8px;
}

.mono {
  font-family: ui-monospace, SFMono-Regular, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  word-break: break-all;
  color: rgba(11, 18, 32, 0.9);
}

.hint {
  font-size: 12px;
  line-height: 1.6;
  color: var(--muted);
}

.url {
  color: rgba(22, 119, 255, 0.85);
}

.actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.restart-hint {
  font-size: 12px;
  line-height: 1.5;
  color: var(--warning);
  padding: 6px 10px;
  border-radius: 8px;
  background: rgba(216, 150, 20, 0.08);
}

.action-bar {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.profile-info {
  display: flex;
  gap: 6px;
  align-items: baseline;
  flex-wrap: wrap;
  font-size: 12px;
}

.profile-info strong {
  color: rgba(11, 18, 32, 0.9);
}

/* ── Step 3 console ── */
.s-status {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: rgba(11, 18, 32, 0.86);
}

.s-dot {
  width: 10px;
  height: 10px;
  border-radius: 999px;
  background: rgba(11, 18, 32, 0.26);
  box-shadow: 0 0 0 4px rgba(11, 18, 32, 0.06);
  flex-shrink: 0;
}
.s-dot[data-status='running'] {
  background: var(--accent-2);
  box-shadow: 0 0 0 4px rgba(19, 194, 194, 0.16);
}
.s-dot[data-status='starting'] {
  background: var(--warning);
  box-shadow: 0 0 0 4px rgba(216, 150, 20, 0.16);
}
.s-dot[data-status='error'] {
  background: var(--danger);
  box-shadow: 0 0 0 4px rgba(212, 56, 13, 0.16);
}

.s-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 16px;
  font-size: 12px;
}
.s-meta-item {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
}
.s-meta-label {
  color: var(--muted);
  font-size: 11px;
}
.s-meta-item strong {
  font-weight: 600;
  color: rgba(11, 18, 32, 0.9);
  word-break: break-all;
}
.s-meta-item[data-tone='error'] strong {
  color: rgba(212, 56, 13, 0.92);
}

.s-health {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 16px;
  border: 1px solid var(--border);
  background: rgba(255, 255, 255, 0.82);
  font-size: 13px;
  color: rgba(11, 18, 32, 0.86);
}
.h-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: var(--muted);
  flex-shrink: 0;
}
.s-health[data-tone='success'] .h-dot { background: var(--accent-2); }
.s-health[data-tone='warning'] .h-dot { background: var(--warning); }

.s-fails {
  display: grid;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 16px;
  border: 1px solid rgba(216, 150, 20, 0.22);
  background: rgba(255, 255, 255, 0.82);
}
.s-fail {
  display: grid;
  gap: 4px;
}
.s-fail strong {
  font-size: 12px;
  color: rgba(11, 18, 32, 0.9);
}
.s-fail p {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: rgba(11, 18, 32, 0.72);
  word-break: break-word;
}

.cmd {
  padding: 10px 12px;
  border-radius: 16px;
  border: 1px dashed rgba(22, 119, 255, 0.28);
  background: rgba(22, 119, 255, 0.06);
  display: grid;
  gap: 6px;
}

.cmd-label {
  font-size: 12px;
  color: rgba(11, 18, 32, 0.72);
  font-weight: 600;
}

/* ── Sandbox ── */
.sandbox {
  display: grid;
  gap: 12px;
}

.sandbox-fields {
  display: grid;
  gap: 12px;
  padding: 4px 0;
}

.sandbox-field {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.sandbox-field-label {
  font-size: 13px;
  font-weight: 500;
  color: rgba(11, 18, 32, 0.88);
  flex-shrink: 0;
}

.sandbox-field-select {
  width: 200px;
}

@media (max-width: 920px) {
  .guide-header {
    flex-direction: column;
  }

  .guide-body {
    grid-template-columns: 1fr;
  }

  .guide-side {
    position: static;
  }
}
</style>
