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
