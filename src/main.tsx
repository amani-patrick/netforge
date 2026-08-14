import React, { useCallback, useRef } from 'react';
import ReactDOM from 'react-dom/client';
import { PacketTracerApp } from './components/PacketTracerApp';
import { useWebSocket, EngineEventHandler } from './hooks/useWebSocket';
import './styles/packet-tracer.css';

const App: React.FC = () => {
  // Multiple components can register event handlers. We fan out to all.
  const handlersRef = useRef<EngineEventHandler[]>([]);

  const handleEngineEvent = useCallback<EngineEventHandler>((event, data) => {
    for (const handler of handlersRef.current) {
      try { handler(event, data); } catch { /* swallow per-handler errors */ }
    }
  }, []);

  const { sendCommand } = useWebSocket('ws://127.0.0.1:8085/ws', handleEngineEvent);

  // Components call this to subscribe to engine events
  const registerHandler = useCallback((handler: EngineEventHandler) => {
    handlersRef.current = [...handlersRef.current.filter((h) => h !== handler), handler];
  }, []);

  return (
    <PacketTracerApp sendCommand={sendCommand} onRegisterHandler={registerHandler} />
  );
};

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
