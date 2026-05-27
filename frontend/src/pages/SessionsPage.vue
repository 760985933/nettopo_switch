<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import {
  CountLegacySessions,
  DeleteCodexSessionBackup,
  GetCodexSessionContent,
  ListCodexSessionBackups,
  ListCodexSessions,
  ListCodexSessionProviders,
  MigrateCodexProviders,
  RestoreCodexSessions,
} from '../../wailsjs/go/main/App'

const { t } = useI18n()
const message = useMessage()
const dialog = useDialog()

const loading = ref(false)
const sessions = ref<CodexSession[]>([])
const legacyCount = ref(0)
const migrating = ref(false)

const selectedSession = ref<SessionDetail | null>(null)
const selectedId = ref<string | null>(null)
const detailLoading = ref(false)

const backups = ref<string[]>([])
const restoringBackup = ref(false)
const deletingBackup = ref(false)
const providers = ref<string[]>([])
const fromProvider = ref('')
const toProvider = ref('')

interface CodexSession {
  id: string
  title: string
  model: string
  modelProvider: string
  messageCount: number
  createdAt: string
  isArchived: boolean
}

interface SessionMessage {
  role: string
  content: string
  timestamp: string
}

interface SessionDetail {
  session: CodexSession
  messages: SessionMessage[]
}

function formatBackupName(path: string): string {
  const parts = path.replace(/\\/g, '/').split('/')
  const name = parts[parts.length - 1] || path
  return name.replace(/^sessions_backup_/, '').replace(/\.tar$/, '')
}

function formatTime(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

async function loadSessions() {
  loading.value = true
  try {
    sessions.value = (await ListCodexSessions()) ?? []
    legacyCount.value = await CountLegacySessions()
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    loading.value = false
  }
}

async function viewSession(row: CodexSession) {
  selectedId.value = row.id
  detailLoading.value = true
  try {
    selectedSession.value = await GetCodexSessionContent(row.id)
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
    selectedSession.value = null
  } finally {
    detailLoading.value = false
  }
}

async function loadBackups() {
  try {
    backups.value = (await ListCodexSessionBackups()) ?? []
  } catch {
    backups.value = []
  }
}

async function confirmRestore(backupPath: string) {
  dialog.warning({
    title: t('sessions.backup.restoreConfirm'),
    content: t('sessions.backup.restoreConfirmContent'),
    positiveText: t('sessions.backup.restoreButton'),
    negativeText: '取消',
    onPositiveClick: async () => {
      await doRestore(backupPath)
    },
  })
}

async function doRestore(backupPath: string) {
  restoringBackup.value = true
  try {
    const result = await RestoreCodexSessions(backupPath)
    if (result.error) {
      message.error(result.error)
      return
    }
    message.success(t('sessions.backup.restoreSuccess', { count: result.migratedCount }))
    await loadSessions()
    await loadBackups()
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    restoringBackup.value = false
  }
}

async function confirmDelete(bp: string) {
  dialog.warning({
    title: '删除备份',
    content: `确定要删除备份 "${formatBackupName(bp)}" 吗？此操作不可撤销。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      await doDelete(bp)
    },
  })
}

async function doDelete(backupPath: string) {
  deletingBackup.value = true
  try {
    await DeleteCodexSessionBackup(backupPath)
    message.success('备份已删除')
    await loadBackups()
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    deletingBackup.value = false
  }
}

async function confirmMigrate() {
  // 加载可用 providers 供用户选择
  providers.value = await ListCodexSessionProviders()
  if (providers.value.length < 2) {
    message.info('没有发现需要迁移的会话（仅有一个 provider）')
    return
  }
  // 默认：from = 第一个非 openai 的 provider, to = openai
  fromProvider.value = providers.value.find(p => p !== 'openai') || providers.value[0]
  toProvider.value = 'openai'

  dialog.warning({
    title: t('sessions.migration.confirmTitle'),
    content: `将会话从 "${fromProvider.value}" 迁移到 "${toProvider.value}"`,
    positiveText: t('sessions.migration.button'),
    negativeText: '取消',
    onPositiveClick: async () => {
      await doMigrate()
    },
  })
}

async function doMigrate() {
  migrating.value = true
  try {
    const result = await MigrateCodexProviders(fromProvider.value, toProvider.value)
    if (result.error) {
      message.error(result.error)
      return
    }
    message.success(t('sessions.migration.success', { count: result.migratedCount, path: result.backupPath }))
    await loadSessions()
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  } finally {
    migrating.value = false
  }
}

function formatMessageContent(content: string): string {
  const escape = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  const html = content
    // 代码块（优先处理）
    .replace(/```(\w*)\n?([\s\S]*?)```/g, (_, lang, code) => {
      const langAttr = lang ? ` class="lang-${escape(lang)}"` : ''
      return `<pre class="msg-code-block"><code${langAttr}>${escape(code)}</code></pre>`
    })
    // 行内代码
    .replace(/`([^`]+)`/g, (_, code) => `<code class="msg-inline-code">${escape(code)}</code>`)
    // 换行转 <br>
    .replace(/\n/g, '<br>')
  return html
}

onMounted(() => {
  loadSessions()
  loadBackups()
})
</script>

<template>
  <div class="sessions-page">
    <div class="page-head">
      <div>
        <h2>{{ t('sessions.title') }}</h2>
        <p>{{ t('sessions.desc') }}</p>
      </div>
      <n-space>
        <n-button secondary :loading="loading" @click="loadSessions">刷新</n-button>
      </n-space>
    </div>

    <!-- Migration banner -->
    <div v-if="legacyCount > 0" class="migration-banner">
      <div class="migration-content">
        <span class="migration-icon">⚠️</span>
        <span>{{ t('sessions.migration.banner', { count: legacyCount }) }}</span>
      </div>
      <n-button type="warning" :loading="migrating" @click="confirmMigrate">
        {{ t('sessions.migration.button') }}
      </n-button>
    </div>

    <!-- Backup management -->
    <div v-if="backups.length > 0" class="backup-section">
      <div class="backup-header">
        <span class="backup-title">{{ t('sessions.backup.title') }}</span>
      </div>
      <div class="backup-list">
        <div v-for="(bp, idx) in backups" :key="idx" class="backup-item">
          <span class="backup-name">{{ formatBackupName(bp) }}</span>
          <n-space>
            <n-button
              size="tiny"
              secondary
              type="warning"
              :loading="restoringBackup"
              @click="confirmRestore(bp)"
            >
              {{ t('sessions.backup.restoreButton') }}
            </n-button>
            <n-button
              size="tiny"
              secondary
              type="error"
              :loading="deletingBackup"
              @click="confirmDelete(bp)"
            >
              删除
            </n-button>
          </n-space>
        </div>
      </div>
    </div>

    <!-- Split panel -->
    <div class="split-panel">
      <!-- Left: Session list -->
      <div class="left-panel">
        <div class="left-panel-head">
          <span class="left-panel-title">{{ t('sessions.title') }}</span>
          <span class="left-panel-count">{{ sessions.length }}</span>
        </div>
        <div v-if="loading" class="panel-loading">
          <n-spin :size="24" />
        </div>
        <template v-else-if="sessions.length > 0">
          <div
            v-for="s in sessions"
            :key="s.id"
            class="session-item"
            :class="{ active: selectedId === s.id }"
            @click="viewSession(s)"
          >
            <div class="session-item-title">{{ s.title || s.id.slice(0, 12) + '…' }}</div>
            <div class="session-item-meta">
              <span>{{ s.model }}</span>
              <span class="meta-dot">·</span>
              <span>{{ s.messageCount }}条</span>
              <span class="meta-dot">·</span>
              <span>{{ formatTime(s.createdAt) }}</span>
              <span v-if="s.isArchived" class="meta-badge archived">{{ t('sessions.status.archived') }}</span>
              <span v-else class="meta-badge active">{{ t('sessions.status.active') }}</span>
            </div>
          </div>
        </template>
        <div v-else class="panel-empty">
          <n-empty :description="t('sessions.empty')" :size="'small'" />
        </div>
      </div>

      <!-- Right: Session detail -->
      <div class="right-panel">
        <div v-if="!selectedId" class="right-panel-placeholder">
          <n-empty :description="'请选择一个会话'" />
        </div>
        <div v-else-if="detailLoading" class="panel-loading">
          <n-spin :description="t('sessions.detail.loading')" :size="28" />
        </div>
        <div v-else-if="selectedSession" class="detail-content">
          <div class="session-meta">
            <n-space vertical :size="4">
              <n-text depth="3">ID: {{ selectedSession.session.id }}</n-text>
              <n-text depth="3">{{ t('sessions.table.model') }}: {{ selectedSession.session.model }}</n-text>
              <n-text depth="3">{{ t('sessions.table.time') }}: {{ formatTime(selectedSession.session.createdAt) }}</n-text>
            </n-space>
          </div>
          <div class="messages-list">
            <div
              v-for="(msg, idx) in selectedSession.messages"
              :key="idx"
              class="message-row"
              :data-role="msg.role"
            >
              <div class="message-role">
                <span class="message-role-label">{{
                  msg.role === 'user' ? t('sessions.detail.roleUser') : t('sessions.detail.roleAssistant')
                }}</span>
                <span class="message-time">{{ formatTime(msg.timestamp) }}</span>
              </div>
              <div class="message-content" v-html="formatMessageContent(msg.content)"></div>
            </div>
            <div v-if="selectedSession.messages && selectedSession.messages.length === 0" class="no-messages">
              <n-text depth="3">{{ t('sessions.detail.noContent') }}</n-text>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.sessions-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-height: 0;
  flex: 1;
}

.page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 18px;
  border-radius: 22px;
  border: 1px solid var(--border);
  background: var(--surface);
  box-shadow: 0 10px 30px rgba(14, 30, 68, 0.08);
}

.page-head h2 {
  margin: 0 0 6px;
  font-size: 18px;
  color: var(--text);
}

.page-head p {
  margin: 0;
  font-size: 12px;
  color: var(--muted);
}

.migration-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 18px;
  border-radius: 18px;
  border: 1px solid rgba(216, 150, 20, 0.3);
  background: rgba(216, 150, 20, 0.06);
}

.migration-content {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  color: var(--text);
}

.migration-icon {
  font-size: 18px;
}

.backup-section {
  border-radius: 18px;
  border: 1px solid var(--border);
  background: var(--surface);
  overflow: hidden;
}

.backup-header {
  padding: 12px 16px 0;
}

.backup-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.backup-list {
  display: grid;
}

.backup-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 16px;
  border-bottom: 1px solid var(--border);
}

.backup-item:last-child {
  border-bottom: none;
}

.backup-name {
  font-size: 13px;
  color: var(--text);
  font-family: monospace;
}

.split-panel {
  display: grid;
  grid-template-columns: 340px 1fr;
  gap: 14px;
  min-height: 0;
  flex: 1;
}

.left-panel {
  border: 1px solid var(--border);
  border-radius: 18px;
  background: var(--surface);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 280px);
}

.left-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border);
}

.left-panel-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.left-panel-count {
  font-size: 11px;
  color: var(--muted);
  background: var(--bg);
  padding: 2px 8px;
  border-radius: 999px;
}

.session-item {
  padding: 12px 16px;
  cursor: pointer;
  border-left: 3px solid transparent;
  border-bottom: 1px solid var(--border);
  transition: background 120ms ease;
}

.session-item:last-child {
  border-bottom: none;
}

.session-item:hover {
  background: rgba(22, 119, 255, 0.03);
}

.session-item.active {
  background: rgba(22, 119, 255, 0.07);
  border-left-color: var(--accent);
}

.session-item-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-bottom: 4px;
}

.session-item-meta {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: var(--muted);
  flex-wrap: wrap;
}

.meta-dot {
  opacity: 0.4;
}

.meta-badge {
  font-size: 10px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: 999px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.meta-badge.active {
  background: rgba(19, 194, 194, 0.12);
  color: var(--accent-2);
}

.meta-badge.archived {
  background: rgba(216, 150, 20, 0.12);
  color: #b8860b;
}

.right-panel {
  border: 1px solid var(--border);
  border-radius: 18px;
  background: var(--surface);
  overflow-y: auto;
  padding: 16px;
  max-height: calc(100vh - 280px);
}

.right-panel-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 300px;
}

.panel-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 60px 0;
}

.panel-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 60px 0;
}

.detail-content {
  display: grid;
  gap: 0;
}

.session-meta {
  padding: 12px 0 16px;
  border-bottom: 1px solid var(--border);
  margin-bottom: 16px;
}

.messages-list {
  display: grid;
  gap: 14px;
  max-height: calc(100vh - 260px);
  overflow-y: auto;
  padding: 4px 0;
}

.message-row {
  padding: 12px 14px;
  border-radius: 14px;
  border: 1px solid var(--border);
  background: var(--surface);
}

.message-row[data-role='assistant'] {
  background: rgba(22, 119, 255, 0.04);
  border-color: rgba(22, 119, 255, 0.12);
}

.message-role {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--accent);
  margin-bottom: 6px;
}

.message-time {
  font-weight: 400;
  text-transform: none;
  letter-spacing: 0;
  opacity: 0.55;
  font-size: 11px;
}

.message-row[data-role='user'] .message-role {
  color: var(--muted);
}

.message-content {
  font-size: 13px;
  line-height: 1.7;
  color: var(--text);
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 400px;
  overflow-y: auto;
}

.no-messages {
  display: flex;
  justify-content: center;
  padding: 40px 0;
}

.message-content :deep(pre.msg-code-block) {
  background: #1e1e2e;
  color: #cdd6f4;
  padding: 12px 14px;
  border-radius: 10px;
  overflow-x: auto;
  font-size: 12px;
  line-height: 1.6;
  margin: 8px 0;
}

.message-content :deep(code.msg-inline-code) {
  background: rgba(22, 119, 255, 0.08);
  color: var(--accent);
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 12px;
  font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
}

@media (max-width: 920px) {
  .page-head {
    flex-direction: column;
    align-items: stretch;
  }

  .migration-banner {
    flex-direction: column;
    align-items: stretch;
  }

  .split-panel {
    grid-template-columns: 1fr;
  }

  .left-panel,
  .right-panel {
    max-height: none;
  }
}
</style>
