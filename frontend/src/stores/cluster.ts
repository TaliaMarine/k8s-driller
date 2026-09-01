import { defineStore } from 'pinia'
import { useEventSource } from '@/composables/useEventSource'
import type { ClusterSummaryDTO } from '@/types/api'

/**
 * Setup-style Pinia store so it can use the useEventSource composable
 * directly (SPECS.md §7.3 realtime client) — one subscription per app
 * lifetime, shared by every component that reads cluster state.
 */
export const useClusterStore = defineStore('cluster', () => {
  const { status, data } = useEventSource<ClusterSummaryDTO>('/api/v1/stream/cluster')
  return { status, summary: data }
})
