export type ProxyStatus = 'stopped' | 'starting' | 'running' | 'error'

export interface Profile {
  id: string
  name: string
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
