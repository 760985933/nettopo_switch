<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ProxyStatusPayload, HealthCheckResult, SourceID } from '../types'

const props = defineProps<{
  source: SourceID
  status: ProxyStatusPayload
  loading: boolean
  health: HealthCheckResult | null
}>()

const emit = defineEmits<{
  health: []
  refresh: []
  stop: []
}>()

const { t } = useI18n()

const statusLabel = computed(() => {
  switch (props.status.status) {
    case 'running': return t('app.status.running')
    case 'starting': return t('app.status.starting')
    case 'error': return t('app.status.error')
    default: return t('app.status.stopped')
  }
})

const healthSummary = computed(() => {
  if (!props.health) return null
  const failed = props.health.checks.filter((item) => !item.ok)
  if (props.health.ok) return { tone: 'success' as const, text: t('console.health.ok') }
  return { tone: 'warning' as const, text: t('console.health.failed', { count: failed.length }) }
})

const failedChecks = computed(() => {
  if (!props.health) return []
  return props.health.checks.filter((item) => !item.ok)
})
</script>

<template>
  <div class="status-section">
    <div class="s-status">
      <span class="s-dot" :data-status="status.status" />
      <span>{{ statusLabel }}</span>
    </div>

    <div class="s-meta">
      <span class="s-meta-item">
        <span class="s-meta-label">{{ t('console.meta.listenAddress') }}:</span>
        <strong>{{ status.listenAddress || t('console.meta.notRunning') }}</strong>
      </span>
      <span class="s-meta-item">
        <span class="s-meta-label">{{ t('console.meta.requestCount') }}:</span>
        <strong>{{ status.requestCount }}</strong>
      </span>
      <span v-if="status.lastError" class="s-meta-item" data-tone="error">
        <span class="s-meta-label">{{ t('console.meta.lastError') }}:</span>
        <strong>{{ status.lastError }}</strong>
      </span>
    </div>

    <div v-if="healthSummary" class="s-health" :data-tone="healthSummary.tone">
      <span class="h-dot" />
      <span>{{ healthSummary.text }}</span>
    </div>
    <div v-if="failedChecks.length" class="s-fails">
      <div v-for="item in failedChecks" :key="item.name" class="s-fail">
        <strong>{{ item.name }}</strong>
        <p>{{ item.message }}</p>
      </div>
    </div>

    <div class="actions">
      <n-button type="primary" :loading="loading" @click="emit('health')">
        {{ t('guide.step.three.healthCheck') }}
      </n-button>
      <n-button secondary :loading="loading" @click="emit('refresh')">
        {{ t('console.actions.refresh') }}
      </n-button>
    </div>
  </div>
</template>

<style scoped>
.status-section {
  display: grid;
  gap: 12px;
}

.s-status {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: rgba(11, 18, 32, 0.86);
}

.s-dot {
  width: 10px;
  height: 10px;
  border-radius: 999px;
  background: rgba(11, 18, 32, 0.26);
  box-shadow: 0 0 0 4px rgba(11, 18, 32, 0.06);
  flex-shrink: 0;
}
.s-dot[data-status='running'] {
  background: var(--accent-2);
  box-shadow: 0 0 0 4px rgba(19, 194, 194, 0.16);
}
.s-dot[data-status='starting'] {
  background: var(--warning);
  box-shadow: 0 0 0 4px rgba(216, 150, 20, 0.16);
}
.s-dot[data-status='error'] {
  background: var(--danger);
  box-shadow: 0 0 0 4px rgba(212, 56, 13, 0.16);
}

.s-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 16px;
  font-size: 12px;
}
.s-meta-item {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
}
.s-meta-label {
  color: var(--muted);
  font-size: 11px;
}
.s-meta-item strong {
  font-weight: 600;
  color: rgba(11, 18, 32, 0.9);
  word-break: break-all;
}
.s-meta-item[data-tone='error'] strong {
  color: rgba(212, 56, 13, 0.92);
}

.s-health {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 16px;
  border: 1px solid var(--border);
  background: rgba(255, 255, 255, 0.82);
  font-size: 13px;
  color: rgba(11, 18, 32, 0.86);
}
.h-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: var(--muted);
  flex-shrink: 0;
}
.s-health[data-tone='success'] .h-dot { background: var(--accent-2); }
.s-health[data-tone='warning'] .h-dot { background: var(--warning); }

.s-fails {
  display: grid;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 16px;
  border: 1px solid rgba(216, 150, 20, 0.22);
  background: rgba(255, 255, 255, 0.82);
}
.s-fail {
  display: grid;
  gap: 4px;
}
.s-fail strong {
  font-size: 12px;
  color: rgba(11, 18, 32, 0.9);
}
.s-fail p {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: rgba(11, 18, 32, 0.72);
  word-break: break-word;
}

.actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
</style>
