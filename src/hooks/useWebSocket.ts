import { useEffect, useRef, useCallback } from 'react';
import { useTopologyStore } from '../store/useTopologyStore';

export type EngineEventHandler = (event: string, data: unknown) => void;

// Maximum delay between reconnect attempts (ms).
const MAX_BACKOFF_MS = 30_000;

export const useWebSocket = (
  url: string = 'ws://127.0.0.1:8085/ws',
  onEngineEvent?: EngineEventHandler
) => {
  const socketRef = useRef<WebSocket | null>(null);
  const handlerRef = useRef(onEngineEvent);
  // Track whether we are deliberately closing so we don't reconnect.
  const intentionalClose = useRef(false);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const backoffMs = useRef(500);

  useEffect(() => {
    handlerRef.current = onEngineEvent;
  }, [onEngineEvent]);

  useEffect(() => {
    intentionalClose.current = false;
    backoffMs.current = 500;

    function connect() {
      const ws = new WebSocket(url);
      socketRef.current = ws;

      ws.onopen = () => {
        console.log('[NetForge] Connected to Go Simulation Core.');
        backoffMs.current = 500; // reset backoff on successful connection
      };

      ws.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data as string);
          if (message.event === 'SIM_TICK') {
            // Suppress tick spam from the console in production; keep for debugging.
            return;
          }
          handlerRef.current?.(message.event, message.data);
        } catch (err) {
          console.error('[NetForge] Failed to parse engine message:', err);
        }
      };

      ws.onerror = () => {
        // Error will always be followed by onclose, so reconnect logic lives there.
      };

      ws.onclose = () => {
        socketRef.current = null;
        if (intentionalClose.current) {
          return;
        }
        const delay = backoffMs.current;
        console.log(`[NetForge] Disconnected. Reconnecting in ${delay}ms…`);
        reconnectTimer.current = setTimeout(() => {
          backoffMs.current = Math.min(backoffMs.current * 2, MAX_BACKOFF_MS);
          connect();
        }, delay);
      };
    }

    connect();

    return () => {
      intentionalClose.current = true;
      if (reconnectTimer.current !== null) {
        clearTimeout(reconnectTimer.current);
        reconnectTimer.current = null;
      }
      socketRef.current?.close();
    };
    // url is the only dep — don't re-run on every render tick
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [url]);

  const sendCommand = useCallback((type: string, payload: unknown) => {
    const ws = socketRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type, payload }));
    } else {
      console.warn('[NetForge] Command dropped — engine not connected:', type);
    }
  }, []);

  return { sendCommand };
};

// Keep updateNodePosition subscribe alive without it being inside the effect.
// The store selector is stable so this won't cause re-renders.
export function useNodePositionSync() {
  return useTopologyStore((state) => state.updateNodePosition);
}
