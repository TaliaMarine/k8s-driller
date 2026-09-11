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

/**
 * Formats millicores the way Kubernetes resource specs are usually read.
 * Rounds to at most one decimal place — values here are often derived from
 * percentage math (e.g. pct/100 * capacity), which can leave a float
 * artifact like 229.999999999997 that must never reach the screen as-is.
 * Math.round (rather than toFixed's string rounding) also naturally drops
 * a trailing ".0" for whole numbers.
 */
export function formatCpu(millicores: number): string {
  if (millicores >= 1000) return `${Math.round(millicores / 100) / 10}`
  return `${Math.round(millicores * 10) / 10}m`
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
