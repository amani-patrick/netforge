import React, { useCallback, useRef } from 'react';
import ReactDOM from 'react-dom/client';
import { PacketTracerApp } from './components/PacketTracerApp';
import { useWebSocket, EngineEventHandler } from './hooks/useWebSocket';
import './styles/packet-tracer.css';

const App: React.FC = () => {
  const engineHandlerRef = useRef<EngineEventHandler | null>(null);

  const handleEngineEvent = useCallback<EngineEventHandler>((event, data) => {
    engineHandlerRef.current?.(event, data);
  }, []);

  const { sendCommand } = useWebSocket('ws://127.0.0.1:8085/ws', handleEngineEvent);

  const registerTerminalHandler = useCallback((handler: EngineEventHandler) => {
    engineHandlerRef.current = handler;
  }, []);

  return (
    <PacketTracerApp sendCommand={sendCommand} onRegisterHandler={registerTerminalHandler} />
  );
};

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
