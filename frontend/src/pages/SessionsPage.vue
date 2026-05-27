<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import type { main } from '../../wailsjs/go/models'
import {
  ArchiveCodexSession,
  CountLegacySessions,
  DeleteCodexSession,
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
const sessions = ref<main.CodexSession[]>([])
const legacyCount = ref(0)
const migrating = ref(false)

const selectedSession = ref<main.SessionDetail | null>(null)
const selectedId = ref<string | null>(null)
const detailLoading = ref(false)

const backups = ref<string[]>([])
const restoringBackup = ref(false)
const deletingBackup = ref(false)
const showBackupModal = ref(false)
const providers = ref<string[]>([])
const fromProvider = ref('')
const toProvider = ref('')

const searchQuery = ref('')
const batchMode = ref(false)
const selectedIds = ref<Set<string>>(new Set())

const filteredSessions = computed(() => {
  if (!searchQuery.value.trim()) return sessions.value
  const q = searchQuery.value.toLowerCase()
  return sessions.value.filter(s =>
    s.title.toLowerCase().includes(q) ||
    s.id.toLowerCase().includes(q) ||
    s.model.toLowerCase().includes(q) ||
    s.modelProvider.toLowerCase().includes(q) ||
    (s.cwd && s.cwd.toLowerCase().includes(q))
  )
})

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

async function copyToClipboard(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    message.success('已复制到剪贴板')
  } catch {
    message.error('复制失败')
  }
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

async function viewSession(row: main.CodexSession) {
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

async function confirmDeleteBackup(bp: string) {
  dialog.warning({
    title: '删除备份',
    content: `确定要删除备份 "${formatBackupName(bp)}" 吗？此操作不可撤销。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      await doDeleteBackup(bp)
    },
  })
}

async function doDeleteBackup(backupPath: string) {
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
  providers.value = await ListCodexSessionProviders()
  if (providers.value.length < 2) {
    message.info('没有发现需要迁移的会话（仅有一个 provider）')
    return
  }
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

function toggleBatchMode() {
  batchMode.value = !batchMode.value
  if (!batchMode.value) selectedIds.value = new Set()
}

function toggleSelect(id: string) {
  const s = new Set(selectedIds.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  selectedIds.value = s
}

function toggleSelectAll() {
  if (selectedIds.value.size === filteredSessions.value.length) {
    selectedIds.value = new Set()
  } else {
    selectedIds.value = new Set(filteredSessions.value.map(s => s.id))
  }
}

async function batchDelete() {
  const count = selectedIds.value.size
  if (count === 0) {
    message.info('请先选择要删除的会话')
    return
  }
  dialog.warning({
    title: '批量删除',
    content: `确定要永久删除 ${count} 个会话吗？此操作不可撤销。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      let ok = 0
      for (const id of selectedIds.value) {
        try {
          await DeleteCodexSession(id)
          ok++
        } catch { /* skip */ }
      }
      message.success(`已删除 ${ok} 个会话`)
      selectedIds.value = new Set()
      batchMode.value = false
      await loadSessions()
      if (selectedId.value && !sessions.value.find(s => s.id === selectedId.value)) {
        selectedId.value = null
        selectedSession.value = null
      }
    },
  })
}

async function confirmDeleteSession(id: string) {
  dialog.warning({
    title: '删除会话',
    content: '确定要永久删除此会话吗？此操作不可撤销。',
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      await DeleteCodexSession(id)
      message.success('会话已删除')
      if (selectedId.value === id) {
        selectedId.value = null
        selectedSession.value = null
      }
      await loadSessions()
    },
  })
}

async function doArchiveSession(id: string) {
  try {
    const updated = await ArchiveCodexSession(id)
    message.success(updated.isArchived ? '会话已归档' : '会话已恢复')
    if (selectedSession.value) {
      selectedSession.value.session.isArchived = updated.isArchived
    }
    await loadSessions()
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  }
}

function formatMessageContent(content: string): string {
  const escape = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  const html = content
    .replace(/```(\w*)\n?([\s\S]*?)```/g, (_, lang, code) => {
      const langAttr = lang ? ` class="lang-${escape(lang)}"` : ''
      return `<pre class="msg-code-block"><code${langAttr}>${escape(code)}</code></pre>`
    })
    .replace(/`([^`]+)`/g, (_, code) => `<code class="msg-inline-code">${escape(code)}</code>`)
    .replace(/\n/g, '<br>')
  return html
}

function openBackupModal() {
  loadBackups()
  showBackupModal.value = true
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

    <!-- Toolbar: search, batch, backup -->
    <div class="toolbar">
      <n-input
        v-model:value="searchQuery"
        placeholder="搜索会话..."
        clearable
        size="small"
        class="search-input"
      >
        <template #prefix>🔍</template>
      </n-input>
      <n-space>
        <n-button
          size="small"
          secondary
          :type="batchMode ? 'primary' : 'default'"
          @click="toggleBatchMode"
        >
          {{ batchMode ? '退出批量' : '批量管理' }}
        </n-button>
        <n-button size="small" secondary @click="openBackupModal">
          备份管理
        </n-button>
      </n-space>
    </div>

    <!-- Split panel -->
    <div class="split-panel">
      <!-- Left: Session list -->
      <div class="left-panel">
        <div class="left-panel-head">
          <span class="left-panel-title">{{ t('sessions.title') }}</span>
          <div class="left-panel-head-right">
            <span v-if="batchMode && selectedIds.size > 0" class="left-panel-count">{{ selectedIds.size }} 已选</span>
            <span v-else class="left-panel-count">{{ filteredSessions.length }}</span>
          </div>
        </div>

        <!-- Batch action bar -->
        <div v-if="batchMode" class="batch-bar">
          <n-button size="tiny" quaternary @click="toggleSelectAll">
            {{ selectedIds.size === filteredSessions.length ? '取消全选' : '全选当前' }}
          </n-button>
          <n-button size="tiny" quaternary @click="selectedIds = new Set()">
            清空已选
          </n-button>
          <n-button size="tiny" quaternary type="error" @click="batchDelete">
            删除 ({{ selectedIds.size }})
          </n-button>
        </div>

        <div v-if="loading" class="panel-loading">
          <n-spin :size="24" />
        </div>
        <template v-else-if="filteredSessions.length > 0">
          <div
            v-for="s in filteredSessions"
            :key="s.id"
            class="session-item"
            :class="{ active: selectedId === s.id }"
            @click="batchMode ? toggleSelect(s.id) : viewSession(s)"
          >
            <div class="session-item-inner">
              <n-checkbox
                v-if="batchMode"
                :checked="selectedIds.has(s.id)"
                class="session-checkbox"
                @click.stop
                @update:checked="toggleSelect(s.id)"
              />
              <div class="session-item-body" @click="batchMode ? toggleSelect(s.id) : viewSession(s)">
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
            </div>
          </div>
        </template>
        <div v-else class="panel-empty">
          <n-empty :description="searchQuery ? '无匹配结果' : t('sessions.empty')" :size="'small'" />
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
          <!-- Detail header: title -->
          <div class="detail-header">
            <div class="detail-title">{{ selectedSession.session.title || selectedSession.session.id.slice(0, 12) + '…' }}</div>
          </div>

          <!-- Detail meta: cwd + resume command -->
          <div class="detail-copy-row">
            <div class="copy-item" @click="copyToClipboard(selectedSession.session.cwd)">
              <span class="copy-label">项目路径</span>
              <span class="copy-value">{{ selectedSession.session.cwd || '-' }}</span>
              <span class="copy-hint">点击复制</span>
            </div>
            <div class="copy-item" @click="copyToClipboard(`claude --resume ${selectedSession.session.id}`)">
              <span class="copy-label">恢复命令</span>
              <code class="copy-value resume-cmd">claude --resume {{ selectedSession.session.id.slice(0, 8) }}…</code>
              <span class="copy-hint">点击复制</span>
            </div>
          </div>

          <!-- Detail meta grid -->
          <div class="detail-meta-grid">
            <div class="meta-field">
              <span class="meta-label">ID</span>
              <span class="meta-val mono">{{ selectedSession.session.id.slice(0, 12) }}…</span>
            </div>
            <div class="meta-field">
              <span class="meta-label">Provider</span>
              <span class="meta-val">{{ selectedSession.session.modelProvider }}</span>
            </div>
            <div class="meta-field">
              <span class="meta-label">模型</span>
              <span class="meta-val">{{ selectedSession.session.model }}</span>
            </div>
            <div class="meta-field">
              <span class="meta-label">时间</span>
              <span class="meta-val">{{ formatTime(selectedSession.session.createdAt) }}</span>
            </div>
          </div>

          <!-- Detail actions -->
          <div class="detail-actions">
            <n-button
              size="tiny"
              secondary
              :type="selectedSession.session.isArchived ? 'primary' : 'default'"
              @click="doArchiveSession(selectedSession.session.id)"
            >
              {{ selectedSession.session.isArchived ? '恢复会话' : '归档会话' }}
            </n-button>
            <n-button
              size="tiny"
              secondary
              type="error"
              @click="confirmDeleteSession(selectedSession.session.id)"
            >
              删除会话
            </n-button>
          </div>

          <!-- Messages -->
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

    <!-- Backup Modal -->
    <n-modal
      v-model:show="showBackupModal"
      title="备份管理"
      preset="card"
      style="width: 520px; max-width: 90vw;"
      :bordered="false"
      :segmented="{ footer: true }"
    >
      <template #header>
        <span class="backup-modal-title">备份管理</span>
      </template>
      <div v-if="backups.length === 0" class="backup-modal-empty">
        <n-empty description="暂无备份" :size="'small'" />
      </div>
      <div v-else class="backup-modal-list">
        <div v-for="(bp, idx) in backups" :key="idx" class="backup-modal-item">
          <div class="backup-modal-info">
            <span class="backup-modal-name">{{ formatBackupName(bp) }}</span>
          </div>
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
              @click="confirmDeleteBackup(bp)"
            >
              删除
            </n-button>
          </n-space>
        </div>
      </div>
    </n-modal>
  </div>
</template>

<style scoped>
.sessions-page {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 0;
  flex: 1;
}

.page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 18px;
  border-radius: 22px;
  border: 1px solid var(--border);
  background: var(--surface);
  box-shadow: 0 10px 30px rgba(14, 30, 68, 0.08);
}

.page-head h2 {
  margin: 0 0 4px;
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
  padding: 12px 18px;
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

/* Toolbar */
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
}

.search-input {
  flex: 1;
}

/* Split panel */
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
  max-height: calc(100vh - 310px);
}

.left-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
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

.batch-bar {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 14px;
  border-bottom: 1px solid var(--border);
  background: rgba(22, 119, 255, 0.03);
}

.session-item {
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
}

.session-item-inner {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 14px;
  cursor: pointer;
  border-left: 3px solid transparent;
}

.session-item.active .session-item-inner {
  border-left-color: var(--accent);
}

.session-checkbox {
  margin-top: 2px;
}

.session-item-body {
  flex: 1;
  min-width: 0;
}

.session-item-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-bottom: 3px;
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
  max-height: calc(100vh - 310px);
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
  padding: 40px 0;
}

/* Detail header */
.detail-header {
  margin-bottom: 12px;
}

.detail-title {
  font-size: 16px;
  font-weight: 700;
  color: var(--text);
  line-height: 1.4;
  word-break: break-word;
}

/* Copy rows */
.detail-copy-row {
  display: grid;
  gap: 8px;
  margin-bottom: 12px;
}

.copy-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 10px;
  border: 1px solid var(--border);
  cursor: pointer;
  transition: background 120ms ease;
}

.copy-item:hover {
  background: rgba(22, 119, 255, 0.04);
  border-color: var(--accent);
}

.copy-label {
  font-size: 11px;
  font-weight: 600;
  color: var(--muted);
  white-space: nowrap;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.copy-value {
  flex: 1;
  font-size: 12px;
  color: var(--text);
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.copy-value.resume-cmd {
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 11px;
  background: rgba(22, 119, 255, 0.06);
  padding: 2px 6px;
  border-radius: 4px;
}

.copy-hint {
  font-size: 10px;
  color: var(--accent);
  opacity: 0;
  transition: opacity 120ms ease;
  white-space: nowrap;
}

.copy-item:hover .copy-hint {
  opacity: 1;
}

/* Meta grid */
.detail-meta-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px;
  margin-bottom: 12px;
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--bg);
}

.meta-field {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.meta-label {
  font-size: 10px;
  font-weight: 600;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.meta-val {
  font-size: 12px;
  color: var(--text);
}

.meta-val.mono {
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 11px;
}

/* Detail actions */
.detail-actions {
  display: flex;
  gap: 8px;
  margin-bottom: 14px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--border);
}

/* Messages */
.detail-content {
  display: grid;
  gap: 0;
}

.messages-list {
  display: grid;
  gap: 14px;
  max-height: calc(100vh - 380px);
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

/* Backup modal */
.backup-modal-title {
  font-size: 15px;
  font-weight: 700;
}

.backup-modal-empty {
  padding: 30px 0;
}

.backup-modal-list {
  display: grid;
  gap: 6px;
}

.backup-modal-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 10px;
  border: 1px solid var(--border);
}

.backup-modal-info {
  flex: 1;
  min-width: 0;
}

.backup-modal-name {
  font-size: 12px;
  color: var(--text);
  font-family: monospace;
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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
