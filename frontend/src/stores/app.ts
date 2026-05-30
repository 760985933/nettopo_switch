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
  StartProxyForSource,
  StopProxyForSource,
  RestartProxyForSource,
  GetProxyStatusForSource,
  RunHealthCheckForSource,
  GetOverviewSnapshotForSource,
} from '../../wailsjs/go/main/App'
import type {
  AppConfig,
  Profile,
  ProxyStatusPayload,
  HealthCheckResult,
  LogEntry,
  OverviewSnapshot,
  UsageStatsResponse,
  SourceID,
  InstanceConfig,
} from '../types'
import { getDefaultProviderPreset, getProviderPreset } from '../utils/providers'

const defaultPreset = getDefaultProviderPreset()

function makeFallbackInstanceConfig(source: SourceID): InstanceConfig {
  return {
    listenHost: '127.0.0.1',
    listenPort: source === 'claude' ? 17420 : 17419,
    requestTimeoutMs: 60000,
    maxRetries: 3,
    mappings: {},
    headers: {},
    currentProfileId: 'default',
    proxyProfileIds: ['default'],
  }
}

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
  proxyProfileIds: [],
  instances: {
    codex: makeFallbackInstanceConfig('codex'),
    claude: makeFallbackInstanceConfig('claude'),
  },
}

function makeFallbackStatus(source: SourceID): ProxyStatusPayload {
  return {
    source,
    status: 'stopped',
    listenAddress: '',
    startedAt: '',
    uptimeSeconds: 0,
    lastError: '',
    requestCount: 0,
  }
}

const FALLBACK_STATUS: ProxyStatusPayload = makeFallbackStatus('codex')

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
function saveAppConfigBridge(cfg: AppConfig): Promise<AppConfig> {
  return SaveAppConfig(cfg as any) as Promise<AppConfig>
}

export const useAppStore = defineStore('app', {
  state: () => ({
    config: { ...FALLBACK_CONFIG } as AppConfig,
    statuses: {
      codex: makeFallbackStatus('codex'),
      claude: makeFallbackStatus('claude'),
    } as Record<SourceID, ProxyStatusPayload>,
    healthChecks: {} as Record<SourceID, HealthCheckResult | null>,
    recentLogs: [] as LogEntry[],
    quickTips: [] as string[],
    usageStats: null as UsageStatsResponse | null,
    isBusy: false,
    lastLoadedAt: '',
  }),
  getters: {
    // Legacy getter (backward compat)
    status(state): ProxyStatusPayload {
      return state.statuses.codex
    },
    healthCheck(state): HealthCheckResult | null {
      return state.healthChecks.codex ?? null
    },
    // Source-aware getters
    statusForSource: (state) => (source: SourceID) => state.statuses[source],
    healthCheckForSource: (state) => (source: SourceID) => state.healthChecks[source] ?? null,
    instanceConfig: (state) => (source: SourceID) => {
      return state.config.instances?.[source] ?? makeFallbackInstanceConfig(source)
    },
    isRunningForSource: (state) => (source: SourceID) => state.statuses[source]?.status === 'running',
    currentProfile(state): Profile | null {
      const p = state.config.profiles[state.config.currentProfileId]
      return p ?? null
    },
    profileList(state): Profile[] {
      return Object.values(state.config.profiles)
    },
    proxyProfiles(state): Profile[] {
      const ids = state.config.proxyProfileIds || []
      return ids.map(id => state.config.profiles[id]).filter(Boolean) as Profile[]
    },
    isRunning(state): boolean {
      return state.statuses.codex.status === 'running'
    },
  },
  actions: {
    async initialize() {
      const snapshot = (await GetOverviewSnapshot()) as unknown as OverviewSnapshot
      this.applySnapshot(snapshot)
    },
    // ── Legacy actions (backward compat, operate on codex) ──
    // These keep the same names as the old API so existing callers don't break.
    async startProxy() {
      return this.startProxyForSource('codex')
    },
    async stopProxy() {
      return this.stopProxyForSource('codex')
    },
    async restartProxy() {
      return this.restartProxyForSource('codex')
    },
    async runHealthCheck() {
      return this.runHealthCheckForSource('codex')
    },
    // ── Source-aware actions (new API) ──
    async refreshStatus(source: SourceID) {
      const status = (await GetProxyStatusForSource(source)) as ProxyStatusPayload
      this.statuses[source] = status
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
    async startProxyForSource(source: SourceID) {
      const status = (await StartProxyForSource(source)) as ProxyStatusPayload
      this.statuses[source] = status
      return status
    },
    async stopProxyForSource(source: SourceID) {
      const status = (await StopProxyForSource(source)) as ProxyStatusPayload
      this.statuses[source] = status
      return status
    },
    async restartProxyForSource(source: SourceID) {
      const status = (await RestartProxyForSource(source)) as ProxyStatusPayload
      this.statuses[source] = status
      return status
    },
    async runHealthCheckForSource(source: SourceID) {
      const result = (await RunHealthCheckForSource(source)) as HealthCheckResult
      this.healthChecks[source] = result
      return result
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
      const ids = this.config.proxyProfileIds || []
      const updated = {
        ...this.config,
        currentProfileId: id,
        profiles: {
          ...this.config.profiles,
          [id]: profile,
        },
        proxyProfileIds: ids.includes(id) ? ids : [...ids, id],
      }
      // Also add to codex instance
      if (updated.instances?.codex) {
        const codexIds = updated.instances.codex.proxyProfileIds || []
        updated.instances = {
          ...updated.instances,
          codex: {
            ...updated.instances.codex,
            proxyProfileIds: codexIds.includes(id) ? codexIds : [...codexIds, id],
          },
        }
      }
      this.config = await saveAppConfigBridge(updated)
      return this.config
    },
    async removeFromProxy(id: string) {
      const ids = (this.config.proxyProfileIds || []).filter(i => i !== id)
      let updated = { ...this.config, proxyProfileIds: ids }
      const profile = this.config.profiles[id]
      if (profile && !profile.apiKey) {
        const { [id]: _, ...rest } = this.config.profiles
        updated = { ...updated, profiles: rest }
        if (id === updated.currentProfileId) {
          const others = Object.keys(rest)
          updated.currentProfileId = others.length > 0 ? others[0] : ''
        }
      }
      this.config = await saveAppConfigBridge(updated)
      return this.config
    },
    async reorderProfiles(orderedIds: string[]) {
      const updated = { ...this.config, proxyProfileIds: orderedIds }
      this.config = await saveAppConfigBridge(updated)
      return this.config
    },
    async reorderAllProfiles(orderedIds: string[]) {
      const reordered: Record<string, Profile> = {}
      for (const id of orderedIds) {
        if (this.config.profiles[id]) {
          reordered[id] = this.config.profiles[id]
        }
      }
      for (const id of Object.keys(this.config.profiles)) {
        if (!reordered[id]) {
          reordered[id] = this.config.profiles[id]
        }
      }
      const updated = { ...this.config, profiles: reordered }
      this.config = await saveAppConfigBridge(updated)
      return this.config
    },
    async deleteProfile(id: string) {
      if (id === this.config.currentProfileId) {
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
    applyStatus(payload: ProxyStatusPayload) {
      const source: SourceID = (payload.source as SourceID) || 'codex'
      this.statuses[source] = payload
    },
    applySnapshot(snapshot: OverviewSnapshot) {
      this.config = snapshot.config
      // Try to populate both statuses from snapshot
      if (snapshot.status.source) {
        const src = snapshot.status.source as SourceID
        this.statuses[src] = snapshot.status
      } else {
        this.statuses.codex = { ...snapshot.status, source: 'codex' }
      }
      this.recentLogs = snapshot.recentLogs
      this.quickTips = snapshot.quickTips
      this.lastLoadedAt = new Date().toISOString()
    },
  },
})
