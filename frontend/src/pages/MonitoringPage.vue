<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import type { UsageStats, ModelStats, TimeSeriesPoint, UsageStatsResponse } from '../types'
import { getProviderPreset } from '../utils/providers'
import UsageStatsCard from '../components/UsageStatsCard.vue'

const { t } = useI18n()
const store = useAppStore()
const loading = ref(false)
const activeTab = ref<'today' | 'week' | 'month' | 'year'>('week')

type TabKey = 'today' | 'week' | 'month' | 'year'

const tabKeyMap: Record<TabKey, keyof UsageStatsResponse> = {
  today: 'today',
  week: 'thisWeek',
  month: 'thisMonth',
  year: 'thisYear',
}

const tabs = computed<TabKey[]>(() => ['today', 'week', 'month', 'year'])

const tabLabels = computed<Record<TabKey, string>>(() => ({
  today: t('monitoring.today'),
  week: t('monitoring.week'),
  month: t('monitoring.month'),
  year: t('monitoring.year'),
}))

const currentStats = computed<UsageStats[]>(() => {
  if (!store.usageStats) return []
  const key = tabKeyMap[activeTab.value]
  return (store.usageStats[key] as UsageStats[]) ?? []
})

const modelStats = computed<ModelStats[]>(() => {
  if (!store.usageStats?.models) return []
  return store.usageStats.models
})

const timeSeries = computed<TimeSeriesPoint[]>(() => {
  if (!store.usageStats?.timeSeries) return []
  return store.usageStats.timeSeries
})

const totalRequests = computed(() =>
  currentStats.value.reduce((sum, s) => sum + s.requestCount, 0)
)

const totalTokens = computed(() =>
  currentStats.value.reduce((sum, s) => sum + s.totalTokens, 0)
)

const totalPromptTokens = computed(() =>
  currentStats.value.reduce((sum, s) => sum + s.promptTokens, 0)
)

const totalCompletionTokens = computed(() =>
  currentStats.value.reduce((sum, s) => sum + s.completionTokens, 0)
)

const successRate = computed(() => {
  const total = currentStats.value.reduce((sum, s) => sum + s.successCount + s.failureCount, 0)
  if (total === 0) return 100
  const success = currentStats.value.reduce((sum, s) => sum + s.successCount, 0)
  return Math.round((success / total) * 100)
})

const avgDuration = computed(() => {
  const total = currentStats.value.reduce((sum, s) => sum + s.requestCount, 0)
  if (total === 0) return 0
  const totalMs = currentStats.value.reduce((sum, s) => sum + s.avgDurationMs * s.requestCount, 0)
  return Math.round(totalMs / total)
})

const maxModelTokens = computed(() => {
  if (modelStats.value.length === 0) return 1
  return Math.max(...modelStats.value.map(m => m.totalTokens), 1)
})

// Line chart state
const hoveredIndex = ref<number | null>(null)
const tooltipLeft = ref(0)
const tooltipTop = ref(0)
const chartBodyRef = ref<HTMLElement | null>(null)

const chartMaxTokens = computed(() => {
  if (timeSeries.value.length === 0) return 1
  const maxPerPoint = timeSeries.value.map(p => Math.max(p.promptTokens, p.completionTokens))
  return Math.max(...maxPerPoint, 1)
})

const normalizedPoints = computed(() => {
  if (timeSeries.value.length === 0) return []
  const n = timeSeries.value.length
  const max = chartMaxTokens.value
  return timeSeries.value.map((p, i) => ({
    date: p.date,
    promptTokens: p.promptTokens,
    completionTokens: p.completionTokens,
    totalTokens: p.totalTokens,
    x: n === 1 ? 50 : (i / (n - 1)) * 100,
    yPrompt: max > 0 ? 100 - (p.promptTokens / max) * 100 : 100,
    yCompletion: max > 0 ? 100 - (p.completionTokens / max) * 100 : 100,
  }))
})

const promptLinePoints = computed(() =>
  normalizedPoints.value.map(p => `${p.x},${p.yPrompt}`).join(' ')
)

const completionLinePoints = computed(() =>
  normalizedPoints.value.map(p => `${p.x},${p.yCompletion}`).join(' ')
)

const gridYSteps = computed(() => [0, 0.25, 0.5, 0.75, 1])

const hoveredPoint = computed(() => {
  if (hoveredIndex.value === null || hoveredIndex.value >= normalizedPoints.value.length) return null
  return normalizedPoints.value[hoveredIndex.value]
})

function onChartMouseMove(e: MouseEvent) {
  const el = chartBodyRef.value
  if (!el || normalizedPoints.value.length === 0) return
  const rect = el.getBoundingClientRect()
  const svgX = ((e.clientX - rect.left) / rect.width) * 100
  let nearest = 0
  let minDist = Infinity
  normalizedPoints.value.forEach((p, i) => {
    const d = Math.abs(p.x - svgX)
    if (d < minDist) { minDist = d; nearest = i }
  })
  hoveredIndex.value = nearest
  tooltipLeft.value = e.clientX - rect.left
  tooltipTop.value = e.clientY - rect.top
}

function onChartMouseLeave() {
  hoveredIndex.value = null
}

function formatDate(dateStr: string): string {
  const d = new Date(dateStr)
  return `${d.getMonth() + 1}/${d.getDate()}`
}

function fullDate(dateStr: string): string {
  const d = new Date(dateStr)
  return d.toLocaleDateString()
}

function modelLabel(m: ModelStats): string {
  if (m.model && m.model !== '') return m.model
  return m.provider || 'unknown'
}

function providerLabel(provider: string): string {
  const preset = getProviderPreset(provider)
  return preset?.label ?? provider
}

async function loadStats() {
  if (loading.value) return
  loading.value = true
  try {
    await store.getUsageStats()
  } catch (err) {
    console.error('Failed to load usage stats:', err)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadStats()
})
</script>

<template>
  <div class="monitoring-page">
    <div class="page-header">
      <h2>{{ t('monitoring.title') }}</h2>
      <n-button size="small" tertiary :loading="loading" @click="loadStats">
        <template #icon><span class="refresh-icon">&#x21bb;</span></template>
      </n-button>
    </div>

    <!-- Time period tabs -->
    <n-tabs v-model:value="activeTab" type="line" animated class="time-tabs">
      <n-tab v-for="tab in tabs" :key="tab" :name="tab" :tab="tabLabels[tab]" />
    </n-tabs>

    <!-- Loading / Empty states -->
    <div v-if="loading" class="loading-state">
      <n-spin size="medium" />
    </div>

    <div v-else-if="totalRequests === 0 && modelStats.length === 0" class="empty-state">
      <div class="empty-icon">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="12" width="3" height="9" rx="1"/><rect x="10" y="7" width="3" height="14" rx="1"/><rect x="17" y="3" width="3" height="18" rx="1"/></svg>
      </div>
      <p>{{ t('monitoring.noData') }}</p>
      <p class="empty-hint">{{ t('monitoring.noDataHint') }}</p>
    </div>

    <template v-else>
      <!-- KPI Summary Cards -->
      <div class="kpi-grid">
        <div class="kpi-card">
          <div class="kpi-icon requests">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
          </div>
          <div class="kpi-body">
            <span class="kpi-value">{{ totalRequests.toLocaleString() }}</span>
            <span class="kpi-label">{{ t('monitoring.requestCount') }}</span>
          </div>
        </div>

        <div class="kpi-card">
          <div class="kpi-icon tokens">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></svg>
          </div>
          <div class="kpi-body">
            <span class="kpi-value">{{ totalTokens.toLocaleString() }}</span>
            <span class="kpi-label">{{ t('monitoring.totalTokens') }}</span>
          </div>
        </div>

        <div class="kpi-card">
          <div class="kpi-icon" :class="successRate >= 95 ? 'success' : successRate >= 80 ? 'warning' : 'danger'">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
          </div>
          <div class="kpi-body">
            <span class="kpi-value">{{ successRate }}<small>%</small></span>
            <span class="kpi-label">{{ t('monitoring.successRate') }}</span>
          </div>
        </div>

        <div class="kpi-card">
          <div class="kpi-icon latency">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="2" x2="12" y2="6"/><line x1="12" y1="18" x2="12" y2="22"/><line x1="4.93" y1="4.93" x2="7.76" y2="7.76"/><line x1="16.24" y1="16.24" x2="19.07" y2="19.07"/><line x1="2" y1="12" x2="6" y2="12"/><line x1="18" y1="12" x2="22" y2="12"/><line x1="4.93" y1="19.07" x2="7.76" y2="16.24"/><line x1="16.24" y1="7.76" x2="19.07" y2="4.93"/></svg>
          </div>
          <div class="kpi-body">
            <span class="kpi-value">{{ avgDuration }}<small>{{ t('monitoring.ms') }}</small></span>
            <span class="kpi-label">{{ t('monitoring.avgDuration') }}</span>
          </div>
        </div>
      </div>

      <!-- Content columns: chart + model breakdown -->
      <div class="content-columns">
        <!-- Time-series line chart -->
        <n-card class="chart-card" :bordered="true" size="small" v-if="timeSeries.length > 0">
          <template #header>
            <span class="card-title">{{ t('monitoring.tokenTrend') }}</span>
          </template>
          <div class="chart-wrap">
            <div class="chart-y-axis">
              <span class="y-tick">{{ chartMaxTokens.toLocaleString() }}</span>
              <span class="y-tick">{{ Math.round(chartMaxTokens / 2).toLocaleString() }}</span>
              <span class="y-tick">0</span>
            </div>
            <div class="chart-body" ref="chartBodyRef" @mousemove="onChartMouseMove" @mouseleave="onChartMouseLeave">
              <svg class="line-chart-svg" viewBox="0 0 100 100" preserveAspectRatio="none">
                <!-- Grid lines -->
                <line
                  v-for="step in gridYSteps"
                  :key="step"
                  x1="0" :y1="step * 100" x2="100" :y2="step * 100"
                  class="chart-grid-line"
                />
                <!-- Prompt tokens line -->
                <polyline
                  v-if="promptLinePoints"
                  :points="promptLinePoints"
                  class="chart-line prompt"
                />
                <!-- Completion tokens line -->
                <polyline
                  v-if="completionLinePoints"
                  :points="completionLinePoints"
                  class="chart-line completion"
                />
                <!-- Hover indicator line -->
                <line
                  v-if="hoveredPoint"
                  :x1="hoveredPoint.x" y1="0" :x2="hoveredPoint.x" y2="100"
                  class="chart-hover-line"
                />
                <!-- Data dots -->
                <circle
                  v-for="(p, idx) in normalizedPoints"
                  :key="'prompt-' + idx"
                  :cx="p.x" :cy="p.yPrompt" r="0.9"
                  class="chart-dot prompt"
                />
                <circle
                  v-for="(p, idx) in normalizedPoints"
                  :key="'completion-' + idx"
                  :cx="p.x" :cy="p.yCompletion" r="0.9"
                  class="chart-dot completion"
                />
                <!-- Hover dots (highlighted) -->
                <circle
                  v-if="hoveredPoint"
                  :cx="hoveredPoint.x" :cy="hoveredPoint.yPrompt" r="1.3"
                  class="chart-dot-hover prompt"
                />
                <circle
                  v-if="hoveredPoint"
                  :cx="hoveredPoint.x" :cy="hoveredPoint.yCompletion" r="1.3"
                  class="chart-dot-hover completion"
                />
              </svg>
              <!-- X-axis labels -->
              <div class="chart-x-labels">
                <span
                  v-for="(p, idx) in normalizedPoints"
                  :key="idx"
                  class="x-label"
                  :style="{ left: p.x + '%' }"
                >{{ formatDate(p.date) }}</span>
              </div>
              <!-- Tooltip -->
              <div
                v-if="hoveredPoint"
                class="chart-tooltip"
                :style="{ left: tooltipLeft + 'px', top: tooltipTop + 'px' }"
              >
                <div class="tooltip-date">{{ fullDate(hoveredPoint.date) }}</div>
                <div class="tooltip-row prompt">
                  <span class="tooltip-dot prompt"/>
                  <span>{{ t('monitoring.promptTokens') }}</span>
                  <strong>{{ hoveredPoint.promptTokens.toLocaleString() }}</strong>
                </div>
                <div class="tooltip-row completion">
                  <span class="tooltip-dot completion"/>
                  <span>{{ t('monitoring.completionTokens') }}</span>
                  <strong>{{ hoveredPoint.completionTokens.toLocaleString() }}</strong>
                </div>
                <div class="tooltip-row total">
                  <span>{{ t('monitoring.totalTokens') }}</span>
                  <strong>{{ hoveredPoint.totalTokens.toLocaleString() }}</strong>
                </div>
              </div>
            </div>
          </div>
          <div class="chart-legend">
            <span class="legend-item"><span class="legend-dot prompt"/>{{ t('monitoring.promptTokens') }}</span>
            <span class="legend-item"><span class="legend-dot completion"/>{{ t('monitoring.completionTokens') }}</span>
          </div>
        </n-card>

        <!-- Model token breakdown -->
        <n-card class="model-card" :bordered="true" size="small" v-if="modelStats.length > 0">
          <template #header>
            <span class="card-title">{{ t('monitoring.modelBreakdown') }}</span>
          </template>
          <div class="model-table">
            <div class="model-header">
              <span class="mcol-model">{{ t('monitoring.model') }}</span>
              <span class="mcol-provider">{{ t('monitoring.provider') }}</span>
              <span class="mcol-tokens">{{ t('monitoring.totalTokens') }}</span>
              <span class="mcol-req">{{ t('monitoring.requestCount') }}</span>
            </div>
            <div v-for="m in modelStats" :key="m.provider + ':' + m.model" class="model-row">
              <span class="mcol-model" :title="m.model || m.provider">{{ modelLabel(m) }}</span>
              <span class="mcol-provider">{{ providerLabel(m.provider) }}</span>
              <span class="mcol-tokens">
                <span class="mtoken-bar-wrap">
                  <span class="mtoken-bar-fill" :style="{ width: (m.totalTokens / maxModelTokens * 100) + '%' }"/>
                </span>
                <span class="mtoken-num">{{ m.totalTokens.toLocaleString() }}</span>
              </span>
              <span class="mcol-req">{{ m.requestCount }}</span>
            </div>
          </div>
        </n-card>
      </div>

      <!-- Prompt vs Completion split summary -->
      <n-card class="split-card" :bordered="true" size="small" v-if="totalTokens > 0">
        <template #header>
          <span class="card-title">{{ t('monitoring.promptCompletionSplit') }}</span>
        </template>
        <div class="split-main">
          <div class="split-bar-large">
            <div
              class="split-fill prompt"
              :style="{ width: totalTokens > 0 ? (totalPromptTokens / totalTokens * 100) + '%' : '0%' }"
            />
            <div
              class="split-fill completion"
              :style="{ width: totalTokens > 0 ? (totalCompletionTokens / totalTokens * 100) + '%' : '0%' }"
            />
          </div>
          <div class="split-legend-row">
            <div class="split-legend-item">
              <span class="legend-dot prompt"/>
              <span>{{ t('monitoring.promptTokens') }}</span>
              <strong>{{ totalPromptTokens.toLocaleString() }}</strong>
            </div>
            <div class="split-legend-item">
              <span class="legend-dot completion"/>
              <span>{{ t('monitoring.completionTokens') }}</span>
              <strong>{{ totalCompletionTokens.toLocaleString() }}</strong>
            </div>
          </div>
        </div>
      </n-card>

      <!-- Provider cards -->
      <div class="provider-section" v-if="currentStats.length > 0">
        <h3 class="section-title">{{ t('monitoring.providerBreakdown') }}</h3>
        <div class="provider-grid">
          <UsageStatsCard
            v-for="s in currentStats"
            :key="s.provider"
            :stats="s"
          />
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.monitoring-page {
  max-width: 1100px;
  margin: 0 auto;
  padding: 8px 0;
}
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}
.page-header h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  color: var(--text);
}
.refresh-icon {
  font-size: 16px;
  line-height: 1;
}
.time-tabs {
  margin-bottom: 20px;
}

/* Loading & empty */
.loading-state {
  display: flex;
  justify-content: center;
  padding: 60px 0;
}
.empty-state {
  text-align: center;
  padding: 60px 20px;
  color: var(--muted);
}
.empty-icon {
  margin-bottom: 12px;
  opacity: 0.4;
}
.empty-state p {
  margin: 0 0 8px;
  font-size: 15px;
}
.empty-hint {
  font-size: 13px;
  opacity: 0.6;
}

/* KPI cards */
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 20px;
}
.kpi-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px 20px;
  border-radius: 12px;
  background: var(--card-bg, #fff);
  border: 1px solid var(--border);
  transition: box-shadow 0.2s;
}
.kpi-card:hover {
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.06);
}
.kpi-icon {
  width: 42px;
  height: 42px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: #fff;
}
.kpi-icon.requests { background: linear-gradient(135deg, #667eea, #764ba2); }
.kpi-icon.tokens { background: linear-gradient(135deg, #1677ff, #13c2c2); }
.kpi-icon.success { background: linear-gradient(135deg, #52c41a, #13c2c2); }
.kpi-icon.warning { background: linear-gradient(135deg, #faad14, #ff7a00); }
.kpi-icon.danger { background: linear-gradient(135deg, #ff4d4f, #ff7a00); }
.kpi-icon.latency { background: linear-gradient(135deg, #f093fb, #f5576c); }
.kpi-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.kpi-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--text);
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
}
.kpi-value small {
  font-size: 14px;
  font-weight: 400;
  color: var(--muted);
  margin-left: 2px;
}
.kpi-label {
  font-size: 11px;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

/* Content columns */
.content-columns {
  display: grid;
  grid-template-columns: 1.4fr 1fr;
  gap: 16px;
  margin-bottom: 20px;
  align-items: start;
}

/* Cards */
.card-title {
  font-weight: 600;
  font-size: 14px;
  color: var(--text);
}

/* Time-series line chart */
.chart-card {
  border-radius: 12px;
}
.chart-wrap {
  display: flex;
  gap: 8px;
  height: 200px;
}
.chart-y-axis {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  padding: 0 0 20px;
  width: 52px;
  flex-shrink: 0;
}
.y-tick {
  font-size: 10px;
  color: var(--muted);
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.chart-body {
  flex: 1;
  min-width: 0;
  position: relative;
  padding-bottom: 20px;
}
.line-chart-svg {
  width: 100%;
  height: 100%;
  display: block;
}
.chart-grid-line {
  stroke: var(--border);
  stroke-width: 0.3;
  vector-effect: non-scaling-stroke;
}
.chart-line {
  fill: none;
  stroke-width: 0.8;
  vector-effect: non-scaling-stroke;
  stroke-linecap: round;
  stroke-linejoin: round;
}
.chart-line.prompt { stroke: #1677ff; }
.chart-line.completion { stroke: #13c2c2; }
.chart-hover-line {
  stroke: var(--muted);
  stroke-width: 0.3;
  stroke-dasharray: 1 1;
  vector-effect: non-scaling-stroke;
}
.chart-dot {
  vector-effect: non-scaling-stroke;
}
.chart-dot.prompt { fill: #1677ff; }
.chart-dot.completion { fill: #13c2c2; }
.chart-dot-hover {
  stroke: #fff;
  stroke-width: 0.4;
  vector-effect: non-scaling-stroke;
}
.chart-dot-hover.prompt { fill: #1677ff; }
.chart-dot-hover.completion { fill: #13c2c2; }

/* X-axis labels */
.chart-x-labels {
  position: relative;
  height: 18px;
  margin-top: 2px;
}
.x-label {
  position: absolute;
  transform: translateX(-50%);
  font-size: 9px;
  color: var(--muted);
  white-space: nowrap;
}

/* Tooltip */
.chart-tooltip {
  position: absolute;
  transform: translate(-50%, calc(-100% - 10px));
  background: var(--card-bg, #fff);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 10px 14px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.1);
  pointer-events: none;
  z-index: 10;
  min-width: 170px;
}
.tooltip-date {
  font-size: 12px;
  font-weight: 600;
  color: var(--text);
  margin-bottom: 6px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--border);
}
.tooltip-row {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: var(--muted);
  margin-bottom: 3px;
}
.tooltip-row:last-child { margin-bottom: 0; }
.tooltip-row.total {
  margin-top: 4px;
  padding-top: 4px;
  border-top: 1px solid var(--border);
}
.tooltip-row strong {
  color: var(--text);
  font-weight: 600;
  margin-left: auto;
  padding-left: 12px;
}
.tooltip-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
  flex-shrink: 0;
}
.tooltip-dot.prompt { background: #1677ff; }
.tooltip-dot.completion { background: #13c2c2; }
.chart-legend {
  display: flex;
  gap: 20px;
  padding-top: 8px;
  border-top: 1px solid var(--border);
}
.legend-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--muted);
}
.legend-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  display: inline-block;
}
.legend-dot.prompt { background: #1677ff; }
.legend-dot.completion { background: #13c2c2; }

/* Model breakdown */
.model-card {
  border-radius: 12px;
}
.model-table {
  display: flex;
  flex-direction: column;
}
.model-header {
  display: grid;
  grid-template-columns: 1fr 80px 1fr 56px;
  gap: 8px;
  padding: 0 0 8px;
  font-size: 10px;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  border-bottom: 1px solid var(--border);
}
.model-row {
  display: grid;
  grid-template-columns: 1fr 80px 1fr 56px;
  gap: 8px;
  align-items: center;
  padding: 8px 0;
  border-bottom: 1px solid var(--border);
  font-size: 12px;
  color: var(--text);
}
.model-row:last-child { border-bottom: none; }

.mcol-model {
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.mcol-provider {
  font-size: 11px;
  color: var(--muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.mcol-tokens {
  display: flex;
  align-items: center;
  gap: 6px;
}
.mtoken-bar-wrap {
  flex: 1;
  height: 6px;
  border-radius: 3px;
  background: var(--border);
  overflow: hidden;
}
.mtoken-bar-fill {
  display: block;
  height: 100%;
  border-radius: 3px;
  background: linear-gradient(90deg, #1677ff, #13c2c2);
  transition: width 0.3s;
}
.mtoken-num {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  font-size: 11px;
  text-align: right;
  min-width: 52px;
}
.mcol-req {
  text-align: right;
  font-variant-numeric: tabular-nums;
  color: var(--muted);
}

/* Prompt/Completion split */
.split-card {
  border-radius: 12px;
  margin-bottom: 20px;
}
.split-main {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.split-bar-large {
  display: flex;
  height: 10px;
  border-radius: 5px;
  overflow: hidden;
  background: var(--border);
}
.split-fill.prompt {
  background: #1677ff;
  transition: width 0.3s;
}
.split-fill.completion {
  background: #13c2c2;
  transition: width 0.3s;
}
.split-legend-row {
  display: flex;
  gap: 32px;
}
.split-legend-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--muted);
}
.split-legend-item strong {
  color: var(--text);
  font-weight: 600;
  margin-left: 2px;
}

/* Provider section */
.provider-section {
  margin-top: 4px;
}
.section-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text);
  margin: 0 0 12px;
}
.provider-grid {
  display: grid;
  gap: 16px;
}

/* Responsive */
@media (max-width: 860px) {
  .kpi-grid {
    grid-template-columns: repeat(2, 1fr);
  }
  .content-columns {
    grid-template-columns: 1fr;
  }
}
</style>
