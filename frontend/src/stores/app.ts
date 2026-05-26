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
  StartProxy,
  StopProxy,
  WriteCodexConfigTomlRaw,
  WriteCodexConfigToml,
} from '../../wailsjs/go/main/App'
import type {
  AppConfig,
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
  defaultModel: 'deepseek-chat',
  requestTimeoutMs: 60000,
  maxRetries: 1,
  enableAutoStart: false,
  minimizeToTray: false,
  logRetentionDays: 7,
  compactMode: true,
  mappings: {},
  headers: {},
}

const FALLBACK_STATUS: ProxyStatusPayload = {
  status: 'stopped',
  listenAddress: '',
  startedAt: '',
  uptimeSeconds: 0,
  lastError: '',
  requestCount: 0,
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
      this.config = (await SaveAppConfig(config)) as AppConfig
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
