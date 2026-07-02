import React, { useState, useRef, useEffect, useCallback } from 'react';
import { useTopologyStore } from '../store/useTopologyStore';
import type { EngineEventHandler } from '../hooks/useWebSocket';

interface TerminalProps {
  sendCommand: (type: string, payload: unknown) => void;
  onRegisterHandler?: (handler: EngineEventHandler) => void;
}

interface CLIResult {
  lines?: string[];
  error?: string;
  success?: boolean;
}

export const Terminal: React.FC<TerminalProps> = ({ sendCommand, onRegisterHandler }) => {
  const selectedNodeId = useTopologyStore((state) => state.selectedNodeId);
  const nodes = useTopologyStore((state) => state.nodes);
  const [history, setHistory] = useState<string[]>([
    'NetForge Virtual IOS Software Engine, Version 1.0.0',
    'Copyright (c) 2026 by NetForge Systems. All rights reserved.',
    '',
    'Full IOS CLI enabled. Examples:',
    '  enable | configure terminal | interface Gi0/0',
    '  router ospf 1 | network 10.0.0.0/24 area 0',
    '  vlan 10 | switchport access vlan 10 | switchport mode trunk',
    '  standby 1 ip 10.0.0.1 | class-map / policy-map / service-policy',
    '  show ip route | show vlan | ping 10.0.0.1 | traceroute 8.8.8.8',
  ]);
  const [input, setInput] = useState<string>('');
  const bottomRef = useRef<HTMLDivElement | null>(null);

  const activeNode = selectedNodeId ? nodes[selectedNodeId] : null;

  const appendHistory = useCallback((lines: string | string[]) => {
    setHistory((prev) => [...prev, ...(Array.isArray(lines) ? lines : [lines])]);
  }, []);

  const handleEngineEvent = useCallback<EngineEventHandler>((event, data) => {
    if (!activeNode) return;
    const payload = data as Record<string, unknown>;

    if (event === 'CLI_RESULT') {
      const result = payload as CLIResult;
      if (result.error) {
        appendHistory(`% ${result.error}`);
      }
      if (result.lines && result.lines.length > 0) {
        appendHistory(result.lines);
      }
    }

    if (event === 'PING_RESULT' && payload.source_id === activeNode.id) {
      if (payload.success) {
        appendHistory(`Success rate is 100 percent (1/1), round-trip min/avg/max = ${payload.rtt_ms}/${payload.rtt_ms}/${payload.rtt_ms} ms`);
      } else {
        appendHistory(`.....  Request timed out. (${payload.message || 'no reply'})`);
      }
    }

    if (event === 'TRACEROUTE_RESULT' && payload.source_id === activeNode.id) {
      const hops = (payload.hops as { hop: number; address: string; rtt: number }[]) || [];
      appendHistory(`Tracing route to ${payload.dest_ip}`);
      for (const h of hops) {
        appendHistory(h.address ? `${h.hop}  ${Math.round(h.rtt)} ms  ${h.address}` : `${h.hop}  *`);
      }
    }
  }, [activeNode, appendHistory]);

  useEffect(() => {
    onRegisterHandler?.(handleEngineEvent);
  }, [handleEngineEvent, onRegisterHandler]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [history]);

  const handleCommandSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const command = input.trim();

    if (!activeNode) {
      appendHistory('>> Error: Select a network node on the canvas to configure it.');
      setInput('');
      return;
    }

    appendHistory(`${activeNode.name}> ${command}`);

    if (command === 'clear') {
      setHistory([]);
    } else if (command.startsWith('ping ')) {
      const targetIP = command.split(/\s+/)[1];
      sendCommand('TRIGGER_PING', { src_id: activeNode.id, dst_ip: targetIP });
    } else if (command.startsWith('traceroute ') || command.startsWith('trace ')) {
      const targetIP = command.split(/\s+/)[1];
      sendCommand('TRACEROUTE', { source_id: activeNode.id, dest_ip: targetIP });
    } else {
      sendCommand('EXEC_CLI', { device_id: activeNode.id, command });
    }

    setInput('');
  };

  return (
    <div className="h-full bg-black text-emerald-400 font-mono p-2 text-xs flex flex-col" style={{ minHeight: 140 }}>
      <div className="border-b border-slate-900 pb-2 mb-2 flex items-center justify-between text-slate-500 font-sans select-none">
        <div>Console Port: {activeNode ? `${activeNode.name} (IOS CLI)` : 'Disconnected'}</div>
        <div className="text-[10px] tracking-widest text-slate-600 uppercase">Full EXEC_CLI Terminal</div>
      </div>

      <div className="flex-1 overflow-y-auto space-y-1 mb-2 scrollbar-thin scrollbar-thumb-slate-800">
        {history.map((line, idx) => (
          <div key={idx} className="whitespace-pre-wrap leading-relaxed">{line}</div>
        ))}
        <div ref={bottomRef} />
      </div>

      <form onSubmit={handleCommandSubmit} className="flex items-center space-x-1 border-t border-slate-900 pt-1.5">
        <span className="text-emerald-500 font-bold select-none">
          {activeNode ? `${activeNode.name}>` : 'NetForge>'}
        </span>
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          className="flex-1 bg-transparent text-emerald-400 focus:outline-none placeholder-emerald-800 caret-emerald-400"
          placeholder={activeNode ? "IOS commands (enable, conf t, show ip route, vlan 10...)" : 'Select a device on the canvas...'}
          disabled={!activeNode}
          autoFocus
        />
      </form>
    </div>
  );
};
