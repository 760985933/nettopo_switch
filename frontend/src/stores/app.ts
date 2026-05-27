import { defineStore } from 'pinia'
import {
  ClearCodexConfigBackups,
  DeleteCodexConfigBackup,
  ExportConfig,
  GenerateCodexConfigToml,
  GetAppConfig,
  GetProxyStatus,
  GetCodexConfigPath,
  GetLogHistory,
  GetOverviewSnapshot,
  ImportConfig,
  ListCodexConfigBackups,
  RestartProxy,
  ReadCodexConfigToml,
  RunHealthCheck,
  RestoreCodexConfigTomlFromBackup,
  RestoreCodexConfigToml,
  SaveAppConfig,
  SetCurrentProfile,
  StartProxy,
  StopProxy,
  WriteCodexConfigTomlRaw,
  WriteCodexConfigToml,
} from '../../wailsjs/go/main/App'
import type {
  AppConfig,
  Profile,
  ProxyStatusPayload,
  HealthCheckResult,
  LogEntry,
  OverviewSnapshot,
} from '../types'

const FALLBACK_CONFIG: AppConfig = {
  listenHost: '127.0.0.1',
  listenPort: 17419,
  deepseekBaseURL: 'https://api.deepseek.com/v1',
  apiKey: '',
  defaultModel: 'deepseek-v4-flash',
  requestTimeoutMs: 60000,
  maxRetries: 1,
  enableAutoStart: false,
  minimizeToTray: false,
  logRetentionDays: 7,
  compactMode: true,
  pluginUnlockEnabled: false,
  mappings: {},
  headers: {},
  currentProfileId: 'default',
  profiles: {},
}

const FALLBACK_STATUS: ProxyStatusPayload = {
  status: 'stopped',
  listenAddress: '',
  startedAt: '',
  uptimeSeconds: 0,
  lastError: '',
  requestCount: 0,
}

function makeDefaultProfile(id: string, name: string): Profile {
  return {
    id,
    name,
    baseURL: 'https://api.deepseek.com/v1',
    apiKey: '',
    defaultModel: 'deepseek-v4-flash',
    requestTimeoutMs: 60000,
    maxRetries: 1,
    mappings: {},
    headers: {},
  }
}

// --- Type-safe Wails bridge ---
// Wails-generated bindings type arguments as Go-derived classes (with methods), but
// at runtime only data fields traverse the JSON serialization boundary.  Plain
// objects matching the field shape are sufficient.  This wrapper isolates the
// unavoidable casts so store actions stay fully type-checked.
function saveAppConfigBridge(cfg: AppConfig): Promise<AppConfig> {
  return SaveAppConfig(cfg as any) as Promise<AppConfig>
}

export const useAppStore = defineStore('app', {
  state: () => ({
    config: { ...FALLBACK_CONFIG } as AppConfig,
    status: { ...FALLBACK_STATUS } as ProxyStatusPayload,
    recentLogs: [] as LogEntry[],
    healthCheck: null as HealthCheckResult | null,
    quickTips: [] as string[],
    isBusy: false,
    lastLoadedAt: '',
  }),
  getters: {
    currentProfile(state): Profile | null {
      const p = state.config.profiles[state.config.currentProfileId]
      return p ?? null
    },
    profileList(state): Profile[] {
      return Object.values(state.config.profiles)
    },
    isRunning(state): boolean {
      return state.status.status === 'running'
    },
  },
  actions: {
    async initialize() {
      const snapshot = (await GetOverviewSnapshot()) as OverviewSnapshot
      this.applySnapshot(snapshot)
    },
    async refreshStatus() {
      this.status = (await GetProxyStatus()) as ProxyStatusPayload
    },
    async refreshConfig() {
      this.config = (await GetAppConfig()) as AppConfig
    },
    async refreshLogs(limit = 200) {
      this.recentLogs = (await GetLogHistory(limit)) as LogEntry[]
    },
    async saveConfig(config: AppConfig) {
      this.config = await saveAppConfigBridge(config)
      return this.config
    },
    async startProxy() {
      this.status = (await StartProxy()) as ProxyStatusPayload
      return this.status
    },
    async stopProxy() {
      this.status = (await StopProxy()) as ProxyStatusPayload
      return this.status
    },
    async restartProxy() {
      this.status = (await RestartProxy()) as ProxyStatusPayload
      return this.status
    },
    async runHealthCheck() {
      this.healthCheck = (await RunHealthCheck()) as HealthCheckResult
      return this.healthCheck
    },
    async exportConfig() {
      return ExportConfig()
    },
    async importConfig(payload: string) {
      this.config = (await ImportConfig(payload)) as AppConfig
      return this.config
    },
    async generateCodexConfigToml() {
      return GenerateCodexConfigToml()
    },
    async writeCodexConfigToml() {
      return WriteCodexConfigToml()
    },
    async getCodexConfigPath() {
      return GetCodexConfigPath()
    },
    async restoreCodexConfigToml() {
      return RestoreCodexConfigToml()
    },
    async listCodexConfigBackups() {
      return ListCodexConfigBackups()
    },
    async deleteCodexConfigBackup(backupPath: string) {
      return DeleteCodexConfigBackup(backupPath)
    },
    async clearCodexConfigBackups() {
      return ClearCodexConfigBackups()
    },
    async restoreCodexConfigTomlFromBackup(backupPath: string) {
      return RestoreCodexConfigTomlFromBackup(backupPath)
    },
    async readCodexConfigToml() {
      return ReadCodexConfigToml()
    },
    async writeCodexConfigTomlRaw(content: string) {
      return WriteCodexConfigTomlRaw(content)
    },
    async setCurrentProfile(id: string) {
      this.config = (await SetCurrentProfile(id)) as AppConfig
      return this.config
    },
    async addProfile(name: string, template?: Profile) {
      const id = 'profile_' + Date.now().toString(36)
      const profile = template
        ? { ...template, id, name }
        : { ...makeDefaultProfile(id, name) }
      const updated = {
        ...this.config,
        currentProfileId: id,
        profiles: {
          ...this.config.profiles,
          [id]: profile,
        },
      }
      this.config = await saveAppConfigBridge(updated)
      return this.config
    },
    async deleteProfile(id: string) {
      if (id === this.config.currentProfileId) {
        // Switch to another profile first
        const others = Object.keys(this.config.profiles).filter((k) => k !== id)
        if (others.length === 0) return this.config
        await this.setCurrentProfile(others[0])
      }
      const { [id]: _, ...rest } = this.config.profiles
      const updated = { ...this.config, profiles: rest }
      this.config = await saveAppConfigBridge(updated)
      return this.config
    },
    pushLog(entry: LogEntry) {
      this.recentLogs = [...this.recentLogs.slice(-199), entry]
    },
    applyStatus(status: ProxyStatusPayload) {
      this.status = status
    },
    applySnapshot(snapshot: OverviewSnapshot) {
      this.config = snapshot.config
      this.status = snapshot.status
      this.recentLogs = snapshot.recentLogs
      this.quickTips = snapshot.quickTips
      this.lastLoadedAt = new Date().toISOString()
    },
  },
})
