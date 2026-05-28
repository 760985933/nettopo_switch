<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { GetUsageBalance } from '../../wailsjs/go/main/App'
import type { UsageBalance } from '../types'

const store = useAppStore()
const { t } = useI18n()

const usageBalance = ref<UsageBalance | null>(null)
const usageLoading = ref(false)

async function fetchUsageBalance() {
  if (!store.currentProfile?.apiKey) {
    usageBalance.value = null
    return
  }
  usageLoading.value = true
  try {
    usageBalance.value = await GetUsageBalance()
  } catch (err) {
    usageBalance.value = { availableBalance: '', totalBalance: '', currency: '', isDepleted: false, error: String(err) }
  } finally {
    usageLoading.value = false
  }
}

function open() {
  fetchUsageBalance()
}

defineExpose({ open })
</script>

<template>
  <div class="monitor-view">
    <div class="monitor-profile-hint">
      {{ t('guide.monitor.viewing', { name: store.currentProfile?.name ?? '-' }) }}
    </div>

    <template v-if="store.currentProfile?.apiKey">
      <div v-if="usageLoading" class="monitor-loading">{{ t('common.loading') }}</div>

      <div v-else-if="usageBalance && !usageBalance.error" class="usage-section">
        <div class="usage-head">
          <span class="usage-title">{{ t('guide.usage.title') }}</span>
          <n-button text size="tiny" :loading="usageLoading" @click="fetchUsageBalance">
            <span class="usage-refresh">↻</span>
          </n-button>
        </div>
        <div class="usage-row">
          <span class="usage-label">{{ t('guide.usage.available') }}:</span>
          <span class="usage-value">{{ usageBalance.availableBalance }} {{ usageBalance.currency }}</span>
          <span class="usage-sep">/</span>
          <span class="usage-label">{{ t('guide.usage.total') }}:</span>
          <span class="usage-value">{{ usageBalance.totalBalance }} {{ usageBalance.currency }}</span>
        </div>
        <div v-if="usageBalance.isDepleted" class="usage-depleted">
          {{ t('guide.usage.depleted') }}
        </div>
      </div>

      <div v-else-if="usageBalance && usageBalance.error" class="usage-section usage-section--error">
        <div class="usage-head">
          <span class="usage-title">{{ t('guide.usage.title') }}</span>
          <n-button text size="tiny" @click="fetchUsageBalance">
            <span class="usage-refresh">↻</span>
          </n-button>
        </div>
        <div class="usage-error">{{ usageBalance.error }}</div>
      </div>

      <div class="monitor-placeholder">
        {{ t('guide.monitor.placeholder') }}
      </div>
    </template>

    <div v-else class="monitor-nokey">
      {{ t('guide.monitor.noKey') }}
    </div>
  </div>
</template>

<style scoped>
.monitor-view {
  display: grid;
  gap: 12px;
}

.monitor-profile-hint {
  font-size: 12px;
  color: var(--muted);
  padding: 4px 0;
}

.monitor-nokey {
  font-size: 13px;
  color: var(--muted);
  padding: 12px 0;
  text-align: center;
}

.monitor-loading {
  font-size: 13px;
  color: var(--muted);
  text-align: center;
  padding: 12px 0;
}

.monitor-placeholder {
  font-size: 12px;
  color: var(--muted);
  text-align: center;
  padding: 16px 0;
  border-top: 1px dashed var(--border);
  opacity: 0.7;
}

/* reuse usage balance styles from QuickGuideCard */
.usage-section {
  padding: 8px 10px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.5);
  border: 1px solid var(--border);
  display: grid;
  gap: 6px;
  font-size: 12px;
}
.usage-section--error {
  opacity: 0.7;
}
.usage-title {
  font-size: 11px;
  font-weight: 600;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.3px;
}
.usage-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.usage-refresh {
  display: inline-block;
  font-size: 14px;
  line-height: 1;
  cursor: pointer;
  opacity: 0.6;
  transition: transform 0.2s, opacity 0.2s;
}
.usage-refresh:hover {
  opacity: 1;
  transform: rotate(180deg);
}
.usage-row {
  display: flex;
  gap: 6px;
  align-items: baseline;
}
.usage-label {
  color: var(--muted);
}
.usage-value {
  font-weight: 600;
  color: rgba(11, 18, 32, 0.9);
}
.usage-sep {
  color: var(--muted);
  margin: 0 2px;
}
.usage-depleted {
  color: rgba(212, 56, 13, 0.92);
  font-weight: 600;
  font-size: 12px;
  padding: 4px 8px;
  border-radius: 8px;
  background: rgba(212, 56, 13, 0.08);
}
.usage-error {
  color: var(--muted);
  word-break: break-word;
  font-size: 11px;
}
</style>
