import { useEffect, useRef, useCallback } from 'react';
import { useTopologyStore } from '../store/useTopologyStore';

export type EngineEventHandler = (event: string, data: unknown) => void;

export const useWebSocket = (
  url: string = 'ws://127.0.0.1:8085/ws',
  onEngineEvent?: EngineEventHandler
) => {
  const socketRef = useRef<WebSocket | null>(null);
  const handlerRef = useRef(onEngineEvent);
  const updateNodePosition = useTopologyStore((state) => state.updateNodePosition);

  useEffect(() => {
    handlerRef.current = onEngineEvent;
  }, [onEngineEvent]);

  useEffect(() => {
    const ws = new WebSocket(url);
    socketRef.current = ws;

    ws.onopen = () => {
      console.log('Connected to NetForge Go Simulation Core Pipeline');
    };

    ws.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data);

        switch (message.event) {
          case 'SIM_TICK':
            console.log(`[Sim Time: ${message.data.Timestamp}ns] Event: ${message.data.Type}`);
            break;
          default:
            handlerRef.current?.(message.event, message.data);
        }
      } catch (err) {
        console.error('Error parsing incoming engine frame payload:', err);
      }
    };

    ws.onclose = () => {
      console.log('Disconnected from Go Core Pipeline');
    };

    return () => {
      ws.close();
    };
  }, [url, updateNodePosition]);

  const sendCommand = useCallback((type: string, payload: unknown) => {
    if (socketRef.current && socketRef.current.readyState === WebSocket.OPEN) {
      socketRef.current.send(JSON.stringify({ type, payload }));
    } else {
      console.error('Cannot emit command: Go Core pipe is currently offline.');
    }
  }, []);

  return { sendCommand };
};
