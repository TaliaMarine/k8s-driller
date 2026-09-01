import { onMounted, onUnmounted, ref, shallowRef } from 'vue'

export type ConnectionStatus = 'connecting' | 'live' | 'reconnecting' | 'disconnected'

const MIN_RETRY_MS = 1000
const MAX_RETRY_MS = 30000

/**
 * Subscribes to one SSE topic (SPECS.md §6.2/§7.3). Every message — snapshot
 * or patch — replaces `data` wholesale; the server always sends a full
 * snapshot on connect, so a fresh reconnect self-heals without any client
 * reconciliation logic.
 */
export function useEventSource<T>(url: string) {
  const status = ref<ConnectionStatus>('connecting')
  const data = shallowRef<T | null>(null)

  let source: EventSource | null = null
  let retryDelay = MIN_RETRY_MS
  let retryTimer: ReturnType<typeof setTimeout> | undefined
  let stopped = false

  function handleMessage(event: MessageEvent<string>) {
    try {
      data.value = JSON.parse(event.data) as T
    } catch {
      // Malformed payload: keep showing the last good state rather than crash.
    }
  }

  function connect() {
    if (stopped) return
    source = new EventSource(url, { withCredentials: true })
    source.addEventListener('snapshot', handleMessage)
    source.addEventListener('patch', handleMessage)
    source.onopen = () => {
      status.value = 'live'
      retryDelay = MIN_RETRY_MS
    }
    source.onerror = () => {
      source?.close()
      if (stopped) return
      status.value = 'reconnecting'
      retryTimer = setTimeout(connect, retryDelay)
      retryDelay = Math.min(retryDelay * 2, MAX_RETRY_MS)
    }
  }

  function disconnect() {
    stopped = true
    if (retryTimer) clearTimeout(retryTimer)
    source?.close()
    status.value = 'disconnected'
  }

  onMounted(connect)
  onUnmounted(disconnect)

  return { status, data }
}
