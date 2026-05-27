<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { useUiStore } from '../stores/ui'
import { GetUsageBalance } from '../../wailsjs/go/main/App'
import type { Profile, ProxyStatusPayload, HealthCheckResult, UsageBalance } from '../types'

const emit = defineEmits<{
  copy: [value: string]
  health: []
  start: []
  stop: []
  restart: []
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

const profileOptions = computed(() =>
  store.profileList.map((p) => ({
    label: p.name,
    value: p.id,
  })),
)

const currentProfileId = computed(() => store.config.currentProfileId)

// Add profile dialog
const showAddDialog = ref(false)
const adding = ref(false)
const newProfileName = ref('')

async function handleSwitchProfile(id: string) {
  if (id === store.config.currentProfileId) return
  try {
    await store.setCurrentProfile(id)
    message.success(t('profile.switched', { name: store.currentProfile?.name ?? id }))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  }
}

async function handleAddProfile() {
  const name = newProfileName.value.trim()
  if (!name) return
  adding.value = true
  try {
    await store.addProfile(name)
    newProfileName.value = ''
    showAddDialog.value = false
    message.success(t('profile.added', { name }))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    adding.value = false
  }
}

function openAddDialog() {
  newProfileName.value = ''
  showAddDialog.value = true
}

async function handleRestoreCodex() {
  try {
    const path = await store.restoreCodexConfigToml()
    message.success(t('settings.toast.restored', { path }))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  }
}

async function handlePluginUnlockChange(val: boolean) {
  try {
    await store.saveConfig({ ...store.config, pluginUnlockEnabled: val })
    message.success(val ? t('guide.step.two.pluginUnlockEnabled') : t('guide.step.two.pluginUnlockDisabled'))
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
    // Silently persist so [sandbox_workspace_write] section always exists in the file
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

// ── Usage balance ──
const usageBalance = ref<UsageBalance | null>(null)
const usageLoading = ref(false)
let usageTimer: ReturnType<typeof setInterval> | null = null

async function fetchUsageBalance() {
  if (!store.currentProfile?.apiKey) {
    usageBalance.value = null
    return
  }
  usageLoading.value = true
  try {
    usageBalance.value = await GetUsageBalance()
  } catch (err) {
    usageBalance.value = { availableBalance: '', totalBalance: '', isDepleted: false, error: String(err) }
  } finally {
    usageLoading.value = false
  }
}

// Re-fetch when profile or apiKey changes (handles initial load after store init)
watch(() => store.currentProfile?.apiKey, (key) => {
  if (key) fetchUsageBalance()
})

onMounted(() => {
  // If store is already loaded, currentProfile will be available and the watch above handles it.
  // If not, the watch fires when the store initializes.
  if (store.currentProfile?.apiKey) fetchUsageBalance()
  usageTimer = setInterval(fetchUsageBalance, 60000)
})

onBeforeUnmount(() => {
  if (usageTimer) clearInterval(usageTimer)
})

async function handleCodexWrite() {
  try {
    const path = await store.writeCodexConfigToml()
    const hintPath = await store.getCodexConfigPath()
    message.success(t('app.toast.codexTomlWritten', { path: path || hintPath }))
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  }
}
</script>

<template>
  <div class="guide-card">
    <div class="guide-header">
      <div>
        <h3>{{ t('guide.title') }}</h3>
      </div>
      <n-button
        tertiary
        type="primary"
        :disabled="!codexBaseURL"
        @click="emit('copy', codexBaseURL)"
      >
        {{ t('guide.actions.copyBaseUrl') }}
      </n-button>
    </div>

    <div class="steps">
      <!-- Step 1: Profile selector + proxy controls -->
      <div class="step">
        <div class="step-head">
          <span class="step-badge">Step 1</span>
          <span class="step-title">{{ t('guide.step.one.title') }}</span>
        </div>
        <div class="step-body">
          <!-- Profile selector -->
          <div class="profile-bar">
            <n-select
              v-model:value="currentProfileId"
              :options="profileOptions"
              :disabled="store.isRunning"
              size="small"
              class="profile-select"
              @update:value="handleSwitchProfile"
            />
            <n-button size="tiny" secondary @click="openAddDialog">
              {{ t('profile.add') }}
            </n-button>
          </div>

          <!-- Proxy action buttons -->
          <div class="action-bar">
            <n-button
              size="small"
              type="primary"
              :disabled="store.isRunning || !currentProfileId"
              :loading="loading"
              @click="emit('start')"
            >
              {{ t('config.actions.start') }}
            </n-button>
            <n-button
              size="small"
              secondary
              :disabled="!store.isRunning"
              :loading="loading"
              @click="emit('restart')"
            >
              {{ t('config.actions.restart') }}
            </n-button>
            <n-button
              size="small"
              tertiary
              type="error"
              :disabled="!store.isRunning"
              :loading="loading"
              @click="emit('stop')"
            >
              {{ t('config.actions.stop') }}
            </n-button>
          </div>

          <!-- Connection info -->
          <div class="mono">{{ props.listenAddress || t('guide.step.one.notRunning') }}</div>
          <div v-if="store.currentProfile" class="profile-info">
            <span class="hint">{{ t('profile.current') }}:</span>
            <strong class="mono">{{ store.currentProfile.name }}</strong>
            <span class="hint">→</span>
            <span class="mono url">{{ store.currentProfile.baseURL }}</span>
          </div>

          <!-- Usage balance -->
          <div v-if="usageBalance && !usageBalance.error" class="usage-section">
            <div class="usage-head">
              <span class="usage-title">{{ t('guide.usage.title') }}</span>
              <n-button text size="tiny" :loading="usageLoading" @click="fetchUsageBalance">
                <span class="usage-refresh">↻</span>
              </n-button>
            </div>
            <div class="usage-rows">
              <div class="usage-row">
                <span class="usage-label">{{ t('guide.usage.available') }}:</span>
                <span class="usage-value">{{ usageBalance.availableBalance }}</span>
              </div>
              <div class="usage-row">
                <span class="usage-label">{{ t('guide.usage.total') }}:</span>
                <span class="usage-value">{{ usageBalance.totalBalance }}</span>
              </div>
            </div>
            <div v-if="usageBalance.isDepleted" class="usage-depleted">
              {{ t('guide.usage.depleted') }}
            </div>
          </div>
          <div v-else-if="usageBalance && usageBalance.error" class="usage-section usage-section--error">
            <div class="usage-head">
              <span class="usage-title">{{ t('guide.usage.title') }}</span>
              <n-button text size="tiny" :loading="usageLoading" @click="fetchUsageBalance">
                <span class="usage-refresh">↻</span>
              </n-button>
            </div>
            <div class="usage-error">{{ usageBalance.error }}</div>
          </div>
        </div>
      </div>

      <!-- Step 2: unchanged -->
      <div class="step">
        <div class="step-head">
          <span class="step-badge">Step 2</span>
          <span class="step-title">{{ t('guide.step.two.title') }}</span>
        </div>
        <div class="step-body">
          <div class="actions">
            <n-button type="primary" @click="handleCodexWrite">{{ t('guide.actions.writeFile') }}</n-button>
            <n-button secondary @click="ui.showSettings = true">{{ t('guide.actions.preferences') }}</n-button>
            <n-button tertiary @click="handleRestoreCodex">{{ t('guide.actions.restoreDefault') }}</n-button>
            <n-button tertiary @click="showSandbox = true">{{ t('guide.actions.sandbox') }}</n-button>
          </div>
          <div class="toggle-row">
            <div class="toggle-row-left">
              <span class="toggle-label">{{ t('guide.step.two.pluginUnlock') }}</span>
            </div>
            <n-switch
              v-model:value="store.config.pluginUnlockEnabled"
              @update:value="handlePluginUnlockChange"
            />
          </div>
          <div class="restart-hint">{{ t('guide.sandbox.configHint') }}</div>
        </div>
      </div>

      <!-- Step 3: merged console + verify -->
      <div class="step">
        <div class="step-head">
          <span class="step-badge">Step 3</span>
          <span class="step-title">{{ t('guide.step.three.title') }}</span>
        </div>
        <div class="step-body">
          <!-- Status indicator -->
          <div class="s-status">
            <span class="s-dot" :data-status="status.status" />
            <span>{{ statusLabel }}</span>
          </div>

          <!-- Meta row -->
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

          <!-- Health result -->
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

          <!-- Actions -->
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
      <n-input
        v-model:value="newProfileName"
        :placeholder="t('profile.namePlaceholder')"
        @keyup.enter="handleAddProfile"
      />
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
        <!-- Switches & selects -->
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

.guide-header h3 {
  margin: 0;
  font-size: 16px;
  color: var(--text);
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

.kv {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  align-items: baseline;
  font-size: 12px;
  color: var(--muted);
}

.kv strong {
  color: rgba(11, 18, 32, 0.9);
  font-weight: 600;
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

.actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.toggle-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 10px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.5);
  border: 1px solid var(--border);
}

.toggle-row-left {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.toggle-label {
  font-size: 13px;
  font-weight: 600;
  color: rgba(11, 18, 32, 0.88);
}

.restart-hint {
  font-size: 12px;
  line-height: 1.5;
  color: var(--warning);
  padding: 6px 10px;
  border-radius: 8px;
  background: rgba(216, 150, 20, 0.08);
}

.profile-bar {
  display: flex;
  gap: 8px;
  align-items: center;
}

.profile-select {
  flex: 1;
  min-width: 0;
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

@media (max-width: 920px) {
  .guide-header {
    flex-direction: column;
  }
}

/* ── merged console (Step 3) ── */
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

/* ── Usage balance ── */
.usage-section {
  padding: 8px 10px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.5);
  border: 1px solid var(--border);
  display: grid;
  gap: 6px;
  font-size: 12px;
}
.usage-section--error {
  opacity: 0.7;
}
.usage-title {
  font-size: 11px;
  font-weight: 600;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.3px;
}
.usage-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.usage-refresh {
  display: inline-block;
  font-size: 14px;
  line-height: 1;
  cursor: pointer;
  opacity: 0.6;
  transition: transform 0.2s, opacity 0.2s;
}
.usage-refresh:hover {
  opacity: 1;
  transform: rotate(180deg);
}
.usage-rows {
  display: grid;
  gap: 3px;
}
.usage-row {
  display: flex;
  gap: 6px;
  align-items: baseline;
}
.usage-label {
  color: var(--muted);
}
.usage-value {
  font-weight: 600;
  color: rgba(11, 18, 32, 0.9);
}
.usage-depleted {
  color: rgba(212, 56, 13, 0.92);
  font-weight: 600;
  font-size: 12px;
  padding: 4px 8px;
  border-radius: 8px;
  background: rgba(212, 56, 13, 0.08);
}
.usage-error {
  color: var(--muted);
  word-break: break-word;
  font-size: 11px;
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
</style>
