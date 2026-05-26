declare module '../../wailsjs/go/main/App' {
 export function ClearCodexConfigBackups(): Promise<number>
 export function DeleteCodexConfigBackup(backupPath: string): Promise<string>
 export function ListCodexConfigBackups(): Promise<string[]>
 export function ReadCodexConfigToml(): Promise<string>
 export function RestoreCodexConfigToml(): Promise<string>
 export function RestoreCodexConfigTomlFromBackup(backupPath: string): Promise<string>
 export function WriteCodexConfigTomlRaw(content: string): Promise<string>
}

declare module '../wailsjs/go/main/App' {
 export function CheckForUpdates(): Promise<main.UpdateCheckResult>
 export function ClearCodexConfigBackups(): Promise<number>
 export function DeleteCodexConfigBackup(backupPath: string): Promise<string>
 export function ExportConfig(): Promise<string>
 export function GenerateCodexConfigToml(): Promise<string>
 export function GetAppConfig(): Promise<main.AppConfig>
 export function GetAppVersion(): Promise<string>
 export function GetProxyStatus(): Promise<main.ProxyStatusPayload>
 export function GetCodexConfigPath(): Promise<string>
 export function GetLogHistory(count: number): Promise<Array<main.LogEntry>>
 export function GetOverviewSnapshot(): Promise<main.OverviewSnapshot>
 export function ImportConfig(configStr: string): Promise<main.AppConfig>
 export function ListCodexConfigBackups(): Promise<string[]>
 export function ReadCodexConfigToml(): Promise<string>
 export function RestartProxy(): Promise<main.ProxyStatusPayload>
 export function RestoreCodexConfigToml(): Promise<string>
 export function RestoreCodexConfigTomlFromBackup(backupPath: string): Promise<string>
 export function RunHealthCheck(): Promise<main.HealthCheckResult>
 export function SaveAppConfig(config: main.AppConfig): Promise<main.AppConfig>
 export function StartProxy(): Promise<main.ProxyStatusPayload>
 export function StopProxy(): Promise<main.ProxyStatusPayload>
 export function WriteCodexConfigToml(): Promise<string>
 export function WriteCodexConfigTomlRaw(content: string): Promise<string>
}

// Augment Window type so generated wailsjs JS files don't error on window['go']
interface Window {
 go: {
 main: {
 App: {
 CheckForUpdates: () => Promise<main.UpdateCheckResult>
 ClearCodexConfigBackups: () => Promise<number>
 DeleteCodexConfigBackup: (arg1: string) => Promise<string>
 ExportConfig: () => Promise<string>
 GenerateCodexConfigToml: () => Promise<string>
 GetAppConfig: () => Promise<main.AppConfig>
 GetAppVersion: () => Promise<string>
 GetProxyStatus: () => Promise<main.ProxyStatusPayload>
 GetCodexConfigPath: () => Promise<string>
 GetLogHistory: (arg1: number) => Promise<Array<main.LogEntry>>
 GetOverviewSnapshot: () => Promise<main.OverviewSnapshot>
 ImportConfig: (arg1: string) => Promise<main.AppConfig>
 ListCodexConfigBackups: () => Promise<string[]>
 ReadCodexConfigToml: () => Promise<string>
 RestartProxy: () => Promise<main.ProxyStatusPayload>
 RestoreCodexConfigToml: () => Promise<string>
 RestoreCodexConfigTomlFromBackup: (arg1: string) => Promise<string>
 RunHealthCheck: () => Promise<main.HealthCheckResult>
 SaveAppConfig: (arg1: main.AppConfig) => Promise<main.AppConfig>
 StartProxy: () => Promise<main.ProxyStatusPayload>
 StopProxy: () => Promise<main.ProxyStatusPayload>
 WriteCodexConfigToml: () => Promise<string>
 WriteCodexConfigTomlRaw: (arg1: string) => Promise<string>
 }
 }
 }
}

// We need the models namespace for type references
declare namespace main {
 export class AppConfig {
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
 mappings: Record<string, string>
 headers: Record<string, string>
 }
 export class ProxyStatusPayload {
 status: string
 listenAddress: string
 startedAt: string
 uptimeSeconds: number
 lastError: string
 requestCount: number
 }
 export class HealthCheckResult {
 ok: boolean
 checks: HealthCheckItem[]
 }
 export class HealthCheckItem {
 name: string
 ok: boolean
 message: string
 }
 export class LogEntry {
 id: string
 level: string
 timestamp: string
 source: string
 message: string
 requestId?: string
 }
 export class OverviewSnapshot {
 config: AppConfig
 status: ProxyStatusPayload
 recentLogs: LogEntry[]
 quickTips: string[]
 defaults: Record<string, string>
 features: Record<string, boolean>
 }
 export class UpdateCheckResult {
 currentVersion: string
 latestVersion: string
 hasUpdate: boolean
 downloadUrl: string
 notes: string
 checkedAt: string
 }
}
