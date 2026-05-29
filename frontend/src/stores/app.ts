import { defineStore } from 'pinia'
import {
  ExportConfig,
  GetAppConfig,
  GetProxyStatus,
  GetLogHistory,
  GetOverviewSnapshot,
  GetUsageStats,
  ImportConfig,
  RestartProxy,
  RunHealthCheck,
  SaveAppConfig,
  SetCurrentProfile,
  StartProxy,
  StopProxy,
} from '../../wailsjs/go/main/App'
import type {
  AppConfig,
  Profile,
  ProxyStatusPayload,
  HealthCheckResult,
  LogEntry,
  OverviewSnapshot,
  UsageStatsResponse,
} from '../types'
import { getDefaultProviderPreset, getProviderPreset } from '../utils/providers'

const defaultPreset = getDefaultProviderPreset()

const FALLBACK_CONFIG: AppConfig = {
  listenHost: '127.0.0.1',
  listenPort: 17419,
  deepseekBaseURL: defaultPreset.defaultBaseURL,
  apiKey: '',
  defaultModel: defaultPreset.defaultModel,
  requestTimeoutMs: 60000,
  maxRetries: 3,
  enableAutoStart: false,
  minimizeToTray: true,
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

function makeDefaultProfile(id: string, name: string, provider?: string): Profile {
  const preset = provider ? getProviderPreset(provider) : undefined
  return {
    id,
    name,
    provider: provider ?? defaultPreset.id,
    baseURL: preset?.defaultBaseURL ?? defaultPreset.defaultBaseURL,
    apiKey: '',
    defaultModel: preset?.defaultModel ?? defaultPreset.defaultModel,
    mappings: {},
    apiType: preset?.apiType ?? defaultPreset.apiType,
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
    usageStats: null as UsageStatsResponse | null,
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
    async setCurrentProfile(id: string) {
      this.config = (await SetCurrentProfile(id)) as AppConfig
      return this.config
    },
    async addProfile(name: string, provider?: string, template?: Profile, apiKey?: string) {
      const id = 'profile_' + Date.now().toString(36)
      const profile = template
        ? { ...template, id, name, apiKey: apiKey || template.apiKey }
        : { ...makeDefaultProfile(id, name, provider), apiKey: apiKey || '' }
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
    async getUsageStats(): Promise<UsageStatsResponse> {
      const stats = (await GetUsageStats()) as UsageStatsResponse
      this.usageStats = stats
      return stats
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
