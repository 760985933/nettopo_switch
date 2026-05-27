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
  MigrateSingleCodexSession,
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

const showSingleMigrateModal = ref(false)
const singleMigrateSessionId = ref('')
const singleMigrateFromProvider = ref('')
const singleMigrateTargetProvider = ref('openai')

const showSearch = ref(false)
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
  providers.value = (await ListCodexSessionProviders()) ?? []
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

async function confirmMigrateSingleSession(id: string, fromProvider: string) {
  const toProvider = 'openai'
  singleMigrateSessionId.value = id
  singleMigrateFromProvider.value = fromProvider
  singleMigrateTargetProvider.value = toProvider
  showSingleMigrateModal.value = true
}

async function doMigrateSingleSession() {
  const id = singleMigrateSessionId.value
  const to = singleMigrateTargetProvider.value
  if (!to) {
    message.error('请输入目标 provider')
    return
  }
  try {
    const updated = await MigrateSingleCodexSession(id, to)
    message.success(t('sessions.migration.singleSuccess') + ' -> ' + to)
    if (selectedSession.value) {
      selectedSession.value.session.modelProvider = updated.modelProvider
    }
    await loadSessions()
    showSingleMigrateModal.value = false
  } catch (error) {
    message.error(error instanceof Error ? error.message : String(error))
  }
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

    <!-- Toolbar: search (toggled), backup -->
    <div class="toolbar">
      <n-input
        v-if="showSearch"
        v-model:value="searchQuery"
        placeholder="搜索会话..."
        clearable
        size="small"
        class="search-input"
      />
      <n-space>
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
          <div class="left-panel-title-row">
            <span class="left-panel-title">{{ t('sessions.title') }}</span>
            <button class="head-icon-btn" :class="{ active: showSearch }" title="搜索" @click="showSearch = !showSearch">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
            </button>
            <button class="head-icon-btn" :class="{ active: batchMode }" title="批量管理" @click="toggleBatchMode">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 11 12 14 22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/></svg>
            </button>
            <button class="head-icon-btn" :class="{ spinning: loading }" title="刷新" @click="loadSessions">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
            </button>
          </div>
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

        <div class="left-panel-body">
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
        </div><!-- /left-panel-body -->
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

          <!-- Detail meta: compact info bar -->
          <div class="detail-info-bar">
            <div class="info-item" @click="copyToClipboard(selectedSession.session.id)">
              <span class="info-label">ID</span>
              <span class="info-val mono">{{ selectedSession.session.id }}</span>
            </div>
            <div class="info-item">
              <span class="info-label">Provider</span>
              <span class="info-val">{{ selectedSession.session.modelProvider }}</span>
            </div>
            <div class="info-item">
              <span class="info-label">模型</span>
              <span class="info-val">{{ selectedSession.session.model }}</span>
            </div>
            <div class="info-item">
              <span class="info-label">时间</span>
              <span class="info-val">{{ formatTime(selectedSession.session.createdAt) }}</span>
            </div>
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
              <code class="copy-value resume-cmd">claude --resume {{ selectedSession.session.id }}</code>
              <span class="copy-hint">点击复制</span>
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
              type="warning"
              @click="confirmMigrateSingleSession(selectedSession.session.id, selectedSession.session.modelProvider)"
            >
              {{ t('sessions.migration.singleButton') }}
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

    <!-- Single Migrate Modal -->
    <n-modal
      v-model:show="showSingleMigrateModal"
      title="迁移会话"
      preset="card"
      style="width: 400px; max-width: 90vw;"
      :bordered="false"
      :segmented="{ footer: true }"
    >
      <template #header>
        <span class="backup-modal-title">迁移会话 Provider</span>
      </template>
      <div style="display: flex; flex-direction: column; gap: 12px;">
        <div style="font-size: 13px; color: var(--text);">
          将会话 provider 从 <code style="font-weight:700;">{{ singleMigrateFromProvider }}</code> 迁移到：
        </div>
        <n-input
          v-model:value="singleMigrateTargetProvider"
          placeholder="输入目标 model_provider"
          clearable
        />
      </div>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 8px;">
          <n-button size="small" @click="showSingleMigrateModal = false">取消</n-button>
          <n-button size="small" type="warning" @click="doMigrateSingleSession">迁移</n-button>
        </div>
      </template>
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

.left-panel-title-row {
  display: flex;
  align-items: center;
  gap: 3px;
}

.left-panel-title-row .head-icon-btn {
  width: 22px;
  height: 22px;
}

.head-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: none;
  border-radius: 6px;
  background: transparent;
  cursor: pointer;
  color: var(--muted);
  transition: all 120ms ease;
}

.head-icon-btn:hover {
  background: rgba(22, 119, 255, 0.08);
  color: var(--accent);
}

.head-icon-btn.active {
  background: rgba(22, 119, 255, 0.12);
  color: var(--accent);
}

.head-icon-btn.spinning svg {
  animation: head-icon-spin 1s linear infinite;
}

@keyframes head-icon-spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
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
  min-height: 0;
}

.left-panel-body {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
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
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  padding: 16px;
  min-height: 0;
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

/* Info bar: compact single-row meta */
.detail-info-bar {
  display: flex;
  gap: 1px;
  margin-bottom: 10px;
  padding: 6px 10px;
  border-radius: 10px;
  background: var(--bg);
  overflow: hidden;
}

.info-item {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 0 10px;
  border-right: 1px solid var(--border);
  cursor: default;
}

.info-item:last-child {
  border-right: none;
}

.info-item:first-child {
  cursor: pointer;
}

.info-item:first-child:hover .info-val {
  color: var(--accent);
}

.info-label {
  font-size: 9px;
  font-weight: 700;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.info-val {
  font-size: 11px;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.info-val.mono {
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 10px;
}

/* Copy rows */
.detail-copy-row {
  display: grid;
  gap: 6px;
  margin-bottom: 10px;
}

.copy-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border-radius: 8px;
  border: 1px solid var(--border);
  cursor: pointer;
  transition: background 120ms ease;
  min-width: 0;
}

.copy-item:hover {
  background: rgba(22, 119, 255, 0.04);
  border-color: var(--accent);
}

.copy-label {
  font-size: 10px;
  font-weight: 700;
  color: var(--muted);
  white-space: nowrap;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  flex-shrink: 0;
}

.copy-value {
  flex: 1;
  font-size: 11px;
  color: var(--text);
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.copy-value.resume-cmd {
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 10px;
  background: rgba(22, 119, 255, 0.06);
  padding: 1px 5px;
  border-radius: 3px;
}

.copy-hint {
  font-size: 9px;
  color: var(--accent);
  opacity: 0;
  transition: opacity 120ms ease;
  white-space: nowrap;
  flex-shrink: 0;
}

.copy-item:hover .copy-hint {
  opacity: 1;
}

/* Detail actions */
.detail-actions {
  display: flex;
  gap: 6px;
  margin-bottom: 10px;
}

/* Messages */
.detail-content {
  display: flex;
  flex-direction: column;
  gap: 0;
  min-height: 0;
  flex: 1;
}

.messages-list {
  display: grid;
  gap: 14px;
  overflow-y: auto;
  padding: 4px 0;
  min-height: 0;
  flex-shrink: 1;
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
/* Scrollbar styles */
.left-panel-body::-webkit-scrollbar,
.right-panel::-webkit-scrollbar,
.messages-list::-webkit-scrollbar {
  width: 6px;
}

.left-panel-body::-webkit-scrollbar-track,
.right-panel::-webkit-scrollbar-track,
.messages-list::-webkit-scrollbar-track {
  background: transparent;
}

.left-panel-body::-webkit-scrollbar-thumb,
.right-panel::-webkit-scrollbar-thumb,
.messages-list::-webkit-scrollbar-thumb {
  background: var(--border);
  border-radius: 3px;
}

.left-panel-body::-webkit-scrollbar-thumb:hover,
.right-panel::-webkit-scrollbar-thumb:hover,
.messages-list::-webkit-scrollbar-thumb:hover {
  background: var(--muted);
}
</style>
