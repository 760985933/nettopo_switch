<script setup lang="ts">
import { computed } from 'vue'
import type { UsageStats } from '../types'
import { getProviderPreset } from '../utils/providers'

const props = defineProps<{
  stats: UsageStats
}>()

const providerInfo = computed(() => {
  const preset = getProviderPreset(props.stats.provider)
  return preset ?? { id: props.stats.provider, label: props.stats.provider, defaultBaseURL: '', defaultModel: '', docsURL: '' }
})

const successRate = computed(() => {
  const total = props.stats.successCount + props.stats.failureCount
  if (total === 0) return 100
  return Math.round((props.stats.successCount / total) * 100)
})

const tokenPercentage = computed(() => {
  const total = props.stats.totalTokens
  if (total === 0) return { prompt: 0, completion: 0 }
  return {
    prompt: Math.round((props.stats.promptTokens / total) * 100),
    completion: Math.round((props.stats.completionTokens / total) * 100),
  }
})

const avgTokensPerRequest = computed(() => {
  if (props.stats.requestCount === 0) return 0
  return Math.round(props.stats.totalTokens / props.stats.requestCount)
})

const rateTagType = computed(() => {
  if (successRate.value >= 95) return 'success'
  if (successRate.value >= 80) return 'warning'
  return 'error'
})
</script>

<template>
  <n-card class="usage-card" :bordered="true" size="small">
    <div class="card-top">
      <div class="provider-name">{{ providerInfo.label }}</div>
      <n-tag :type="rateTagType" size="small" :bordered="false">
        {{ successRate }}%
      </n-tag>
    </div>

    <div class="metrics-row">
      <div class="metric">
        <span class="metric-val">{{ stats.requestCount }}</span>
        <span class="metric-lbl">{{ $t('monitoring.requestCount') }}</span>
      </div>
      <div class="metric">
        <span class="metric-val ok">{{ stats.successCount }}</span>
        <span class="metric-lbl">{{ $t('monitoring.successCount') }}</span>
      </div>
      <div class="metric">
        <span class="metric-val fail">{{ stats.failureCount }}</span>
        <span class="metric-lbl">{{ $t('monitoring.failureCount') }}</span>
      </div>
      <div class="metric">
        <span class="metric-val">{{ stats.avgDurationMs.toFixed(0) }}<small>ms</small></span>
        <span class="metric-lbl">{{ $t('monitoring.avgDuration') }}</span>
      </div>
    </div>

    <div v-if="stats.totalTokens > 0" class="token-section">
      <div class="token-head">
        <span>{{ $t('monitoring.totalTokens') }}</span>
        <strong>{{ stats.totalTokens.toLocaleString() }}</strong>
      </div>
      <div class="token-bar">
        <div
          class="tok-fill prompt"
          :style="{ width: tokenPercentage.prompt + '%' }"
        />
        <div
          class="tok-fill completion"
          :style="{ width: tokenPercentage.completion + '%' }"
        />
      </div>
      <div class="token-foot">
        <span class="tok-info">
          <span class="dot prompt"/> {{ $t('monitoring.promptTokens') }} <strong>{{ stats.promptTokens.toLocaleString() }}</strong>
        </span>
        <span class="tok-info">
          <span class="dot completion"/> {{ $t('monitoring.completionTokens') }} <strong>{{ stats.completionTokens.toLocaleString() }}</strong>
        </span>
        <span class="tok-avg">{{ $t('monitoring.avgTokensPerRequest') }} <strong>{{ avgTokensPerRequest.toLocaleString() }}</strong></span>
      </div>
    </div>

    <div v-else class="token-none">
      {{ $t('monitoring.noData') }}
    </div>
  </n-card>
</template>

<style scoped>
.usage-card {
  border-radius: 12px;
}
.card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
}
.provider-name {
  font-weight: 700;
  font-size: 15px;
  color: var(--text);
}
.metrics-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
  margin-bottom: 14px;
}
.metric {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
}
.metric-val {
  font-size: 18px;
  font-weight: 700;
  color: var(--text);
  font-variant-numeric: tabular-nums;
}
.metric-val small {
  font-size: 11px;
  font-weight: 400;
  color: var(--muted);
  margin-left: 1px;
}
.metric-val.ok { color: #52c41a; }
.metric-val.fail { color: #ff4d4f; }
.metric-lbl {
  font-size: 10px;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.token-section {
  border-top: 1px solid var(--border);
  padding-top: 12px;
}
.token-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
  font-size: 12px;
  color: var(--muted);
}
.token-head strong {
  color: var(--text);
  font-size: 14px;
}
.token-bar {
  display: flex;
  height: 6px;
  border-radius: 3px;
  overflow: hidden;
  background: var(--border);
  margin-bottom: 8px;
}
.tok-fill.prompt { background: #1677ff; transition: width 0.3s; }
.tok-fill.completion { background: #13c2c2; transition: width 0.3s; }
.token-foot {
  display: flex;
  gap: 16px;
  font-size: 11px;
  color: var(--muted);
  flex-wrap: wrap;
  align-items: center;
}
.tok-info {
  display: flex;
  align-items: center;
  gap: 4px;
}
.tok-info strong {
  color: var(--text);
  font-weight: 600;
}
.tok-avg {
  margin-left: auto;
}
.tok-avg strong {
  color: var(--text);
  font-weight: 600;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
  flex-shrink: 0;
}
.dot.prompt { background: #1677ff; }
.dot.completion { background: #13c2c2; }
.token-none {
  border-top: 1px solid var(--border);
  padding-top: 12px;
  text-align: center;
  font-size: 12px;
  color: var(--muted);
}
</style>
