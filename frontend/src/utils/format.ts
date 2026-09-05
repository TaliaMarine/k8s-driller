import type { NodeHealth } from '@/types/api'

/**
 * Maps a node's health label to the shared severity theme color (SPECS.md
 * §7.2). Overcommit is a configuration risk, not an immediate one — it only
 * bites if pods actually use their limit — so it reads as warning, not
 * critical. Critical is reserved for CPU/Mem Pressure (live usage already
 * over 90% of capacity, a problem happening right now).
 */
export function nodeHealthColor(health: NodeHealth): string {
  switch (health) {
    case 'Healthy':
      return 'healthy'
    case 'Overcommit':
      return 'warning'
    case 'Not Ready':
      return 'watch'
    case 'Unschedulable':
      return 'warning'
    default:
      return 'critical'
  }
}

/** Formats millicores the way Kubernetes resource specs are usually read. */
export function formatCpu(millicores: number): string {
  if (millicores >= 1000) return `${(millicores / 1000).toFixed(millicores % 1000 === 0 ? 0 : 1)}`
  return `${millicores}m`
}

/** Formats bytes as binary Ki/Mi/Gi, matching Kubernetes resource units. */
export function formatMem(bytes: number): string {
  const units = ['B', 'Ki', 'Mi', 'Gi', 'Ti']
  let value = bytes
  let unitIndex = 0
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex++
  }
  return `${value.toFixed(unitIndex === 0 ? 0 : 1)}${units[unitIndex]}`
}
