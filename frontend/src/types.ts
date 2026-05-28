export type ProxyStatus = 'stopped' | 'starting' | 'running' | 'error'

export interface Profile {
  id: string
  name: string
  provider: string
  baseURL: string
  apiKey: string
  defaultModel: string
  requestTimeoutMs: number
  maxRetries: number
  mappings: Record<string, string>
  headers: Record<string, string>
}

export interface AppConfig {
  listenHost: string
  listenPort: number
  deepseekBaseURL: string
  apiKey: string
  defaultModel: string
  requestTimeoutMs: number
  maxRetries: number
  enableAutoStart: boolean
  minimizeToTray: boolean
  logRetentionDays: number
  compactMode: boolean
  pluginUnlockEnabled: boolean
  mappings: Record<string, string>
  headers: Record<string, string>
  currentProfileId: string
  profiles: Record<string, Profile>
}

export interface ProxyStatusPayload {
  status: ProxyStatus
  listenAddress: string
  startedAt: string
  uptimeSeconds: number
  lastError: string
  requestCount: number
}

export interface LogEntry {
  id: string
  level: 'info' | 'warn' | 'error'
  timestamp: string
  source: 'app' | 'proxy' | 'healthcheck' | string
  message: string
  requestId?: string
}

export interface HealthCheckItem {
  name: string
  ok: boolean
  message: string
}

export interface HealthCheckResult {
  ok: boolean
  checks: HealthCheckItem[]
}

export interface OverviewSnapshot {
  config: AppConfig
  status: ProxyStatusPayload
  recentLogs: LogEntry[]
  quickTips: string[]
  defaults: Record<string, string>
  features: Record<string, boolean>
}

export interface SandboxWorkspaceConfig {
  networkAccess: boolean
  sandboxMode: string
  approvalPolicy: string
}

export interface UsageBalance {
  availableBalance: string
  totalBalance: string
  currency: string
  isDepleted: boolean
  error?: string
}

// Session sync types
export interface CodexSession {
  id: string
  title: string
  model: string
  modelProvider: string
  messageCount: number
  createdAt: string
  isArchived: boolean
  cwd: string
}

export interface SessionMessage {
  role: string
  content: string
  timestamp: string
}

export interface SessionDetail {
  session: CodexSession
  messages: SessionMessage[]
}

export interface MigrationResult {
  migratedCount: number
  backupPath: string
  error?: string
}

export interface ProviderCounts {
  [provider: string]: number
}

export interface SyncRolloutInfo {
  sessions: ProviderCounts
  archivedSessions: ProviderCounts
}

export interface SyncRepairStats {
  userEventRowsNeedingRepair: number
  cwdRowsNeedingRepair: number
}

export interface ProjectThreadInfo {
  root: string
  interactiveThreads: number
  firstPageThreads: number
  exactCwdMatches: number
  verbatimCwdRows: number
  topRank: number
  ranks: number[]
  rankPreview: string
  providerCounts: ProviderCounts
}

export interface SyncStatusResult {
  codexHome: string
  currentProvider: string
  currentProviderImplicit: boolean
  configuredProviders: string[]
  rolloutCounts: SyncRolloutInfo
  lockedRolloutFiles: string[]
  encryptedContentCounts?: SyncRolloutInfo
  encryptedContentWarning?: string
  sqliteCounts?: SyncRolloutInfo
  sqliteUnreadable: boolean
  sqliteError?: string
  sqliteRepairStats?: SyncRepairStats
  projectThreadVisibility: ProjectThreadInfo[]
  backupRoot: string
  backupCount: number
}

export interface SyncResult {
  codexHome: string
  targetProvider: string
  previousProvider: string
  backupDir: string
  backupDurationMs: number
  changedSessionFiles: number
  skippedLockedFiles: string[]
  sqliteRowsUpdated: number
  sqliteProviderRowsUpdated: number
  sqliteUserEventRowsUpdated: number
  sqliteCwdRowsUpdated: number
  updatedWorkspaceRoots: number
  savedWorkspaceRootCount: number
  sqlitePresent: boolean
  encryptedContentWarning?: string
}
