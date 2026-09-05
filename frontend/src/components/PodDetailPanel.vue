<script setup lang="ts">
import { ref } from 'vue'
import type { PodAnalysisDTO, PodDTO } from '@/types/api'
import { formatCpu, formatMem } from '@/utils/format'
import DeltaBars from './DeltaBars.vue'
import UsageHistoryChart from './UsageHistoryChart.vue'

const props = defineProps<{ pod: PodDTO }>()

const tab = ref('charts')

function containerAllocation(
  pod: PodDTO,
  field: 'requestsCpu' | 'requestsMem' | 'limitsCpu' | 'limitsMem',
) {
  return pod.containers.reduce((sum, c) => sum + (c[field] ?? 0), 0)
}

// --- Analysis tab: fetched on demand, not on mount, since it drives a
// Prometheus range query over up to a month of history per pod (SPECS.md
// §9/§10 — Prometheus-derived features are opt-in-per-view, not part of the
// always-on live path). ---

const analysis = ref<PodAnalysisDTO | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)

async function runAnalysis() {
  loading.value = true
  error.value = null
  try {
    const res = await fetch(
      `/api/v1/pods/${encodeURIComponent(props.pod.namespace)}/${encodeURIComponent(props.pod.name)}/analysis?days=30`,
      { credentials: 'include' },
    )
    if (!res.ok) {
      error.value =
        res.status === 404
          ? 'Prometheus is unavailable, or has no history for this pod yet.'
          : `Analysis request failed (${res.status}).`
      analysis.value = null
      return
    }
    analysis.value = await res.json()
  } catch {
    error.value = 'Analysis request failed.'
    analysis.value = null
  } finally {
    loading.value = false
  }
}

function exportForAI() {
  if (!analysis.value) return
  const payload = {
    about:
      "Exported from Kubernetes Driller (https://github.com/TaliaMarine/k8s-driller), an open-source Kubernetes resource-pressure dashboard. This file contains one pod's current resource requests/limits, raw historical CPU (millicores) and memory (bytes) usage samples pulled from Prometheus, computed statistics (mean/median/min/max/p90/p95/p99/stddev/coefficient of variation), and algorithmically derived request/limit recommendations with a plain-language rationale for each. The recommendation approach is modeled on the Kubernetes Vertical Pod Autoscaler recommender: a high usage percentile rather than the mean, weighted by how variable usage is, plus extra safety margin for memory since under-provisioning it risks an unrecoverable OOM-kill. Use this data to advise on right-sizing this workload's CPU/memory requests and limits.",
    exportedAt: new Date().toISOString(),
    pod: {
      namespace: props.pod.namespace,
      name: props.pod.name,
      nodeName: props.pod.nodeName,
      phase: props.pod.phase,
      controller: props.pod.controller,
      containers: props.pod.containers,
    },
    analysis: analysis.value,
  }
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `${props.pod.namespace}-${props.pod.name}-analysis.json`
  link.click()
  URL.revokeObjectURL(url)
}
</script>

<template>
  <v-row no-gutters class="pod-detail-panel">
    <v-col cols="auto" class="pod-detail-tabs">
      <v-tabs v-model="tab" direction="vertical" density="compact" color="watch">
        <v-tab value="charts" prepend-icon="mdi-chart-bar">Charts</v-tab>
        <v-tab value="analysis" prepend-icon="mdi-magnify-scan">Analysis</v-tab>
      </v-tabs>
    </v-col>

    <v-col class="pod-detail-content">
      <v-window v-model="tab">
        <v-window-item value="charts">
          <DeltaBars
            label="CPU"
            :usage="pod.usageCpu"
            :request="containerAllocation(pod, 'requestsCpu') || undefined"
            :limit="containerAllocation(pod, 'limitsCpu') || undefined"
            :format="formatCpu"
            :danger="pod.throttlingRisk"
          />
          <DeltaBars
            label="Memory"
            :usage="pod.usageMem"
            :request="containerAllocation(pod, 'requestsMem') || undefined"
            :limit="containerAllocation(pod, 'limitsMem') || undefined"
            :format="formatMem"
            :danger="pod.oomRisk"
          />
        </v-window-item>

        <v-window-item value="analysis">
          <div class="d-flex align-center ga-3 mb-4">
            <v-btn color="watch" variant="tonal" :loading="loading" @click="runAnalysis">
              Analyse
            </v-btn>
            <span class="text-caption text-medium-emphasis">
              Pulls up to 30 days of Prometheus history and recommends requests/limits.
            </span>
          </div>

          <v-alert v-if="error" type="warning" variant="tonal" density="compact" class="mb-4">
            {{ error }}
          </v-alert>

          <template v-if="analysis">
            <div class="text-caption text-medium-emphasis mb-3">
              Analysed {{ analysis.availableDays.toFixed(1) }} day{{
                analysis.availableDays === 1 ? '' : 's'
              }}
              of history
              <template v-if="analysis.availableDays < analysis.requestedDays - 0.5">
                (less than the requested {{ analysis.requestedDays }} — Prometheus doesn't have more
                yet)</template
              >.
            </div>

            <UsageHistoryChart
              label="CPU usage"
              :samples="analysis.cpuSamples"
              :request-value="analysis.currentRequestCpu"
              :limit-value="analysis.currentLimitCpu"
              :format="formatCpu"
            />
            <UsageHistoryChart
              label="Memory usage"
              :samples="analysis.memSamples"
              :request-value="analysis.currentRequestMem"
              :limit-value="analysis.currentLimitMem"
              :format="formatMem"
            />

            <v-table density="compact" class="mb-4 stats-table">
              <thead>
                <tr>
                  <th></th>
                  <th class="text-right">Avg</th>
                  <th class="text-right">Median</th>
                  <th class="text-right">Min</th>
                  <th class="text-right">Max</th>
                  <th class="text-right">P90</th>
                  <th class="text-right">Current req.</th>
                  <th class="text-right">Current limit</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td class="font-weight-medium">CPU</td>
                  <td class="text-right">{{ formatCpu(analysis.cpuStats.mean) }}</td>
                  <td class="text-right">{{ formatCpu(analysis.cpuStats.median) }}</td>
                  <td class="text-right">{{ formatCpu(analysis.cpuStats.min) }}</td>
                  <td class="text-right">{{ formatCpu(analysis.cpuStats.max) }}</td>
                  <td class="text-right">{{ formatCpu(analysis.cpuStats.p90) }}</td>
                  <td class="text-right">
                    {{
                      analysis.currentRequestCpu != null
                        ? formatCpu(analysis.currentRequestCpu)
                        : '—'
                    }}
                  </td>
                  <td class="text-right">
                    {{
                      analysis.currentLimitCpu != null ? formatCpu(analysis.currentLimitCpu) : '—'
                    }}
                  </td>
                </tr>
                <tr>
                  <td class="font-weight-medium">Memory</td>
                  <td class="text-right">{{ formatMem(analysis.memStats.mean) }}</td>
                  <td class="text-right">{{ formatMem(analysis.memStats.median) }}</td>
                  <td class="text-right">{{ formatMem(analysis.memStats.min) }}</td>
                  <td class="text-right">{{ formatMem(analysis.memStats.max) }}</td>
                  <td class="text-right">{{ formatMem(analysis.memStats.p90) }}</td>
                  <td class="text-right">
                    {{
                      analysis.currentRequestMem != null
                        ? formatMem(analysis.currentRequestMem)
                        : '—'
                    }}
                  </td>
                  <td class="text-right">
                    {{
                      analysis.currentLimitMem != null ? formatMem(analysis.currentLimitMem) : '—'
                    }}
                  </td>
                </tr>
              </tbody>
            </v-table>

            <div class="d-flex flex-wrap ga-2 mb-4">
              <v-chip v-if="analysis.wasteful" color="warning" size="small" variant="tonal">
                <v-icon start icon="mdi-trending-down" />
                Wasteful — configured well above observed usage
              </v-chip>
              <v-chip
                v-if="analysis.underProvisioned"
                color="critical"
                size="small"
                variant="tonal"
              >
                <v-icon start icon="mdi-trending-up" />
                Under-provisioned — usage runs above the current request
              </v-chip>
            </div>

            <v-row>
              <v-col cols="12" md="6">
                <v-card variant="tonal" density="compact">
                  <v-card-text>
                    <div class="text-caption text-medium-emphasis mb-1">Recommended CPU</div>
                    <div class="text-body-1 mb-1">
                      Request {{ formatCpu(analysis.cpuRecommendation.recommendedRequest) }} · Limit
                      {{ formatCpu(analysis.cpuRecommendation.recommendedLimit) }}
                      <span class="text-medium-emphasis">
                        (now requests
                        {{
                          analysis.currentRequestCpu != null
                            ? formatCpu(analysis.currentRequestCpu)
                            : '—'
                        }}
                        limit
                        {{
                          analysis.currentLimitCpu != null
                            ? formatCpu(analysis.currentLimitCpu)
                            : '—'
                        }})
                      </span>
                    </div>
                    <div class="text-caption text-medium-emphasis">
                      {{ analysis.cpuRecommendation.rationale }}
                    </div>
                  </v-card-text>
                </v-card>
              </v-col>
              <v-col cols="12" md="6">
                <v-card variant="tonal" density="compact">
                  <v-card-text>
                    <div class="text-caption text-medium-emphasis mb-1">Recommended Memory</div>
                    <div class="text-body-1 mb-1">
                      Request {{ formatMem(analysis.memRecommendation.recommendedRequest) }} · Limit
                      {{ formatMem(analysis.memRecommendation.recommendedLimit) }}
                      <span class="text-medium-emphasis">
                        (now requests
                        {{
                          analysis.currentRequestMem != null
                            ? formatMem(analysis.currentRequestMem)
                            : '—'
                        }}
                        limit
                        {{
                          analysis.currentLimitMem != null
                            ? formatMem(analysis.currentLimitMem)
                            : '—'
                        }})
                      </span>
                    </div>
                    <div class="text-caption text-medium-emphasis">
                      {{ analysis.memRecommendation.rationale }}
                    </div>
                  </v-card-text>
                </v-card>
              </v-col>
            </v-row>

            <v-btn
              class="mt-4"
              variant="outlined"
              prepend-icon="mdi-download"
              size="small"
              @click="exportForAI"
            >
              Export raw data for AI
            </v-btn>
          </template>
        </v-window-item>
      </v-window>
    </v-col>
  </v-row>
</template>

<style scoped>
.pod-detail-tabs {
  border-right: 1px solid rgba(var(--v-theme-on-surface), 0.1);
  margin-right: 16px;
}
.pod-detail-content {
  min-width: 0;
}
.stats-table {
  background: transparent;
}
</style>
