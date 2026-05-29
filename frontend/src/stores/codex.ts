import { defineStore } from 'pinia'
import {
  ClearCodexConfigBackups,
  DeleteCodexConfigBackup,
  GenerateCodexConfigToml,
  GenerateCodexConfigTomlProfiles,
  GetCodexConfigPath,
  ListCodexConfigBackups,
  PluginUnlockLogin,
  ReadCodexConfigToml,
  RestoreCodexConfigToml,
  RestoreCodexConfigTomlFromBackup,
  WriteCodexConfigToml,
  WriteCodexConfigTomlProfiles,
  WriteCodexConfigTomlRaw,
  GetSandboxConfig,
  SetSandboxConfig,
} from '../../wailsjs/go/main/App'
import type { SandboxWorkspaceConfig } from '../types'

export const useCodexStore = defineStore('codex', {
  actions: {
    async generateCodexConfigToml() {
      return GenerateCodexConfigToml()
    },
    async writeCodexConfigToml() {
      return WriteCodexConfigToml()
    },
    async writeCodexConfigTomlProfiles() {
      return WriteCodexConfigTomlProfiles()
    },
    async generateCodexConfigTomlProfiles() {
      return GenerateCodexConfigTomlProfiles()
    },
    async pluginUnlockLogin() {
      return PluginUnlockLogin()
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
    async getSandboxConfig(): Promise<SandboxWorkspaceConfig> {
      return GetSandboxConfig()
    },
    async setSandboxConfig(cfg: SandboxWorkspaceConfig): Promise<SandboxWorkspaceConfig> {
      return SetSandboxConfig(cfg)
    },
  },
})
