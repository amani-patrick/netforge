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
    const payload = data as Record<string, unknown>;

    if (event === 'CLI_RESULT') {
      const result = payload as CLIResult;
      if (result.error) appendHistory(`% ${result.error}`);
      if (result.lines && result.lines.length > 0) appendHistory(result.lines);
      return;
    }

    if (event === 'PING_RESULT') {
      if (activeNode && payload.source_id === activeNode.id) {
        appendHistory(payload.success
          ? `Success rate is 100 percent (1/1), round-trip min/avg/max = ${payload.rtt_ms}/${payload.rtt_ms}/${payload.rtt_ms} ms`
          : `.....  Request timed out. (${payload.message || 'no reply'})`
        );
      }
      return;
    }

    if (event === 'TRACEROUTE_RESULT') {
      const hops = (payload.hops as { hop: number; address: string; rtt: number }[]) || [];
      appendHistory(`Tracing route to ${payload.dest_ip}:`);
      for (const h of hops) {
        appendHistory(h.address ? `  ${h.hop}  ${Math.round(h.rtt)} ms  ${h.address}` : `  ${h.hop}  *`);
      }
      appendHistory('Trace complete.');
      return;
    }

    if (event === 'ROUTE_TABLE') {
      const rows = (payload.routes as { network?: string; protocol?: string; next_hop?: string }[]) ?? [];
      appendHistory(`Routing table for ${payload.router_id}:`);
      appendHistory('Codes: C - connected, S - static, O - OSPF, R - RIP, B - BGP');
      for (const r of rows) {
        appendHistory(`  ${(r.protocol ?? 'C').padEnd(2)} ${r.network ?? '?'}${r.next_hop ? '  via ' + r.next_hop : ''}`);
      }
      if (rows.length === 0) appendHistory('  (no routes)');
      return;
    }

    if (event === 'OSPF_NEIGHBORS') {
      const neighbors = (payload.neighbors as { neighbor_id?: string; state?: string; interface?: string }[]) ?? [];
      appendHistory(`OSPF neighbors for ${payload.router_id}:`);
      for (const n of neighbors) {
        appendHistory(`  ${n.neighbor_id ?? '?'}  ${(n.state ?? 'DOWN').padEnd(10)}  ${n.interface ?? ''}`);
      }
      if (neighbors.length === 0) appendHistory('  (no neighbors)');
      return;
    }

    if (event === 'VLAN_TABLE') {
      const rows = (data as { id?: number; name?: string }[]) ?? [];
      appendHistory('VLAN  Name');
      appendHistory('----  ----------------');
      for (const v of rows) appendHistory(`${String(v.id ?? '').padEnd(4)}  ${v.name ?? ''}`);
      return;
    }

    if (event === 'HSRP_STATUS') {
      appendHistory('HSRP Status:');
      appendHistory(JSON.stringify(data, null, 2).split('\n'));
      return;
    }

    if (event === 'CRYPTO_SA') {
      appendHistory('Crypto Security Associations:');
      appendHistory(JSON.stringify(data, null, 2).split('\n'));
      return;
    }

    if (event === 'IKE_ESTABLISHED') {
      appendHistory(`IKE Phase 1 established with peer ${payload.peer} on router ${payload.router_id}`);
      return;
    }

    if (event === 'IKE_ERROR') {
      appendHistory(`% IKE negotiation failed: ${payload.error}`);
      return;
    }

    if (event === 'QOS_POLICY') {
      appendHistory('QoS Policy:');
      appendHistory(JSON.stringify(data, null, 2).split('\n'));
      return;
    }

    if (event === 'DHCP_ASSIGNED') {
      appendHistory(`DHCP: ${payload.host_id} assigned ${payload.ip} (gw: ${payload.gateway}, mask: ${payload.mask}, dns: ${payload.dns})`);
      return;
    }

    if (event === 'DNS_RESULT') {
      appendHistory(payload.found
        ? `DNS: ${payload.name} → ${payload.ip}`
        : `% DNS: ${payload.name} not found`
      );
      return;
    }

    if (event === 'STATIC_ROUTE_ADDED') {
      appendHistory(`Static route added: ${payload.cidr} on ${payload.router_id}`);
      return;
    }

    if (event === 'OSPF_CONFIGURED') appendHistory(`OSPF configured on ${payload.router_id}`);
    if (event === 'RIP_CONFIGURED') appendHistory(`RIP configured on ${payload.router_id}`);

    if (event === 'VOIP_CALLS') {
      const calls = (data as { call_id?: string; from_phone?: string; to_extension?: string; state?: string }[]) ?? [];
      appendHistory('Active VoIP Calls:');
      for (const c of calls) appendHistory(`  ${c.call_id ?? '?'}  ${c.from_phone ?? '?'} → ${c.to_extension ?? '?'}  [${c.state ?? '?'}]`);
      if (calls.length === 0) appendHistory('  (none)');
      return;
    }

    if (event === 'SIP_CALL_STARTED') {
      appendHistory(`SIP Call initiated: ${JSON.stringify(data)}`);
      return;
    }

    if (event === 'SCCP_REGISTERED') {
      appendHistory(`SCCP: Phone ${payload.phone_id} registered with ${payload.cm_id}`);
      return;
    }

    if (event === '5G_STATUS') {
      appendHistory('5G/LTE Gateway Status:');
      appendHistory(JSON.stringify(data, null, 2).split('\n'));
      return;
    }

    if (event === '5G_ATTACHED') {
      appendHistory(`5G NR: UE ${payload.ue_id} attached to ${payload.gateway_id} @ ${payload.ip} (band ${payload.band})`);
      return;
    }

    if (event === 'VPN_LAB_LOADED') {
      appendHistory('VPN Lab loaded. See crypto map / NEGOTIATE_IKE for IPsec config.');
      return;
    }

    if (event === 'TOPOLOGY_SAVED') appendHistory(`Topology saved to ${payload.path}`);
    if (event === 'TOPOLOGY_LOADED') appendHistory(`Topology loaded from ${payload.path}`);
    if (event === 'TOPOLOGY_EXPORT') appendHistory(`Topology exported (JSON data sent to Inspect Panel).`);

    if (event === 'CONFIG_SAVED') appendHistory(`Configuration written: ${payload.device_id}`);

    if (event === 'DEVICE_LIST') {
      const devices = (data as { id?: string; type?: string; ports?: string[] }[]) ?? [];
      appendHistory('Device Inventory:');
      for (const d of devices) appendHistory(`  ${(d.id ?? '?').padEnd(16)} ${(d.type ?? '?').padEnd(12)} ports: ${(d.ports ?? []).join(', ')}`);
      return;
    }

    if (event === 'ACTIVITY_RESULTS') {
      const results = (data as { id?: string; passed?: boolean; message?: string }[]) ?? [];
      appendHistory('Activity Assessment:');
      for (const r of results) {
        appendHistory(`  ${r.passed ? '✓' : '✗'} [${r.id ?? '?'}] ${r.message ?? ''}`);
      }
      return;
    }

    if (event === 'EVENT_LOG') {
      const evs = (data as { category?: string; node_id?: string; message?: string }[]) ?? [];
      appendHistory(`Event Log (${evs.length} entries):`);
      for (const e of evs) appendHistory(`  [${e.category ?? '?'}] ${e.node_id ?? '?'}: ${e.message ?? ''}`);
      return;
    }

    if (event === 'PORT_CAPTURE') {
      const frames = (data as { src_mac?: string; dst_mac?: string; type?: string }[]) ?? [];
      appendHistory(`Port Capture (${frames.length} frames):`);
      for (const f of frames) appendHistory(`  ${f.src_mac ?? '?'} → ${f.dst_mac ?? '?'}  [${f.type ?? '?'}]`);
      return;
    }

    if (event === 'IPV6_ROUTE_TABLE') {
      const rows = (payload.routes as { network?: string; next_hop?: string }[]) ?? [];
      appendHistory(`IPv6 Routing table for ${payload.router_id}:`);
      for (const r of rows) appendHistory(`  ${r.network ?? '?'}${r.next_hop ? '  via ' + r.next_hop : ''}`);
      if (rows.length === 0) appendHistory('  (no IPv6 routes)');
      return;
    }

    if (event === 'CDP_NEIGHBORS') {
      const neighbors = (payload.neighbors as { neighbor_id?: string; interface?: string; platform?: string }[] | Record<string, unknown>) ?? [];
      appendHistory(`CDP Neighbors for ${payload.router_id}:`);
      if (Array.isArray(neighbors)) {
        for (const n of neighbors) appendHistory(`  ${n.neighbor_id ?? '?'}  ${n.interface ?? '?'}`);
        if (neighbors.length === 0) appendHistory('  (none)');
      } else {
        appendHistory(JSON.stringify(neighbors, null, 2).split('\n'));
      }
      return;
    }

    if (event === 'ERROR') {
      appendHistory(`% Error: ${payload.message ?? JSON.stringify(data)}`);
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
