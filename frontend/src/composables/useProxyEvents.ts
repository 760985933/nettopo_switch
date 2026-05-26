import { onBeforeUnmount, onMounted } from 'vue'
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime'
import type { ProxyStatusPayload, LogEntry } from '../types'

interface ProxyEventHandlers {
  onStatus?: (payload: ProxyStatusPayload) => void
  onLog?: (entry: LogEntry) => void
}

export function useProxyEvents(handlers: ProxyEventHandlers): void {
  onMounted(() => {
    if (handlers.onStatus) {
      void EventsOn('proxy:status', handlers.onStatus)
    }
    if (handlers.onLog) {
      void EventsOn('log:entry', handlers.onLog)
    }
  })

  onBeforeUnmount(() => {
    void EventsOff('proxy:status')
    void EventsOff('log:entry')
  })
}
