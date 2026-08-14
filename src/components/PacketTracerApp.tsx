import React, { useCallback, useState } from 'react';
import { useTopologyStore, NodeType } from '../store/useTopologyStore';
import { DevicePalette } from './DevicePalette';
import { TopologyCanvas } from './TopologyCanvas';
import { Terminal } from './Terminal';
import { DeviceCategory, DeviceModel } from '../assets/deviceCatalog';
import type { EngineEventHandler } from '../hooks/useWebSocket';
import { useKeyboardShortcuts, useDeleteSelection } from '../hooks/useKeyboardShortcuts';

interface PacketTracerAppProps {
  sendCommand: (type: string, payload: unknown) => void;
  onRegisterHandler?: (handler: EngineEventHandler) => void;
}

function buildRouterInterfaces(routerIndex: number) {
  return [{
    port_id: 'GigabitEthernet0/1',
    ip: `192.168.${routerIndex}.1`,
    mask: '255.255.255.0',
    mac: `00:1A:2B:3C:${routerIndex.toString(16).padStart(2, '0')}:02`,
  }];
}

function mapModelToNodeType(model: DeviceModel): NodeType | null {
  switch (model) {
    case 'ROUTER': return 'ROUTER';
    case 'SWITCH': return 'SWITCH';
    case 'PC': return 'PC';
    case 'SERVER': return 'SERVER';
    case 'ASA': return 'ASA';
    case 'AP': return 'AP';
    case 'PHONE': return 'PHONE';
    case 'CLOUD': return 'CLOUD';
    case 'COPPER':
    case 'FIBER':
      return null;
    default: return 'ROUTER';
  }
}

export const PacketTracerApp: React.FC<PacketTracerAppProps> = ({ sendCommand, onRegisterHandler }) => {
  const [category, setCategory] = useState<DeviceCategory>('routers');
  const {
    linkMode, setLinkMode, activeTool, setActiveTool,
    simMode, setSimMode, bottomTab, setBottomTab,
    nodes, setPendingPlacement,
  } = useTopologyStore();

  // Keyboard: Delete/Backspace removes selected node or link.
  const handleDelete = useDeleteSelection(sendCommand);

  // Keyboard: Escape cancels link mode, pending placement, or resets to select tool.
  const handleEscape = useCallback(() => {
    setLinkMode(false);
    setPendingPlacement(null);
    setActiveTool('select');
  }, [setLinkMode, setPendingPlacement, setActiveTool]);

  useKeyboardShortcuts(handleDelete, handleEscape);

  const spawnDevice = useCallback((type: NodeType, label: string, x: number, y: number) => {
    const node = useTopologyStore.getState().addNode(type, label, x, y);
    const state = useTopologyStore.getState();

    if (type === 'ROUTER') {
      const routerIndex = Object.values(state.nodes).filter((n) => n.type === 'ROUTER').length;
      sendCommand('ADD_ROUTER', { id: node.id, interfaces: buildRouterInterfaces(routerIndex) });
    } else if (type === 'SWITCH') {
      sendCommand('ADD_SWITCH', { id: node.id });
    } else if (type === 'PC' || type === 'SERVER') {
      const idx = Object.values(state.nodes).filter((n) => n.type === 'PC' || n.type === 'SERVER').length;
      sendCommand('ADD_HOST', {
        id: node.id, ip: `10.0.${idx}.10`, mask: '255.255.255.0',
        gateway: `10.0.${idx}.1`, mac: node.macAddress,
      });
    } else if (type === 'ASA') {
      sendCommand('ADD_ASA', { id: node.id });
    } else if (type === 'AP') {
      sendCommand('ADD_AP', { id: node.id, ssid: 'NetForge', password: 'cisco123' });
    } else if (type === 'PHONE') {
      sendCommand('ADD_VOIP_PHONE', { id: node.id });
    }
  }, [sendCommand]);

  const handleDevicePick = (model: DeviceModel, label: string) => {
    if (model === 'COPPER' || model === 'FIBER') {
      setLinkMode(true);
      return;
    }
    const type = mapModelToNodeType(model);
    if (!type) return;
    const cx = 400 + Math.random() * 200;
    const cy = 200 + Math.random() * 150;
    spawnDevice(type, label, cx, cy);
  };

  const loadVPNLab = () => {
    sendCommand('LOAD_VPN_LAB', {});
    spawnDevice('ROUTER', 'HQ', 280, 180);
    spawnDevice('ROUTER', 'Branch', 520, 180);
    spawnDevice('PC', 'HQ-PC', 280, 320);
    spawnDevice('PC', 'Branch-PC', 520, 320);
    spawnDevice('CLOUD', 'Internet', 400, 80);
    const state = useTopologyStore.getState();
    const ids = Object.keys(state.nodes);
    if (ids.length >= 2) {
      const link = state.connectNodes(ids[0], 'Gi0/0', ids[1], 'Gi0/0', 'fiber');
      sendCommand('CONNECT_LINK', {
        id: link.id,
        source_node_id: ids[0],
        source_port_id: 'Gi0/0',
        target_node_id: ids[1],
        target_port_id: 'Gi0/0',
        bandwidth: 10000000,
      });
    }
  };

  const tools: { id: typeof activeTool; icon: string; title: string }[] = [
    { id: 'select', icon: '↖', title: 'Select' },
    { id: 'inspect', icon: '🔍', title: 'Inspect' },
    { id: 'delete', icon: '✕', title: 'Delete' },
    { id: 'note', icon: '📝', title: 'Place Note' },
    { id: 'move', icon: '✥', title: 'Move Object' },
  ];

  return (
    <div className="pt-app">
      <div className="pt-menubar">
        <span className="pt-title">NetForge Packet Tracer</span>
        <span>File</span><span>Edit</span><span>View</span><span>Options</span><span>Tools</span>
        <span>Extensions</span><span>Window</span><span>Help</span>
        <span style={{ marginLeft: 'auto', fontSize: 10, opacity: 0.8 }}>Logical | Physical</span>
      </div>

      <div className="pt-toolbar">
        {tools.map((t) => (
          <button
            key={t.id}
            type="button"
            className={`pt-tool-btn ${activeTool === t.id ? 'active' : ''}`}
            title={t.title}
            onClick={() => { setActiveTool(t.id); if (t.id === 'select') setLinkMode(false); }}
          >
            {t.icon}
          </button>
        ))}
        <span style={{ marginLeft: 12, color: '#555' }}>|</span>
        <button type="button" className="pt-tool-btn" title="VPN Site-to-Site Lab" onClick={loadVPNLab} style={{ width: 'auto', padding: '0 8px', fontSize: 11 }}>
          VPN Lab
        </button>
        <button
          type="button"
          className={`pt-tool-btn ${linkMode ? 'active' : ''}`}
          title="Connections"
          onClick={() => setLinkMode(!linkMode)}
          style={{ width: 'auto', padding: '0 8px', fontSize: 11 }}
        >
          🔗 Connect
        </button>
      </div>

      <div className="pt-main">
        <div className="pt-workspace-wrap">
          <TopologyCanvas sendCommand={sendCommand} onSpawnDevice={spawnDevice} />
          <DevicePalette
            category={category}
            onCategoryChange={setCategory}
            onDevicePick={handleDevicePick}
            linkMode={linkMode}
          />
        </div>
        <div className="pt-right-toolbar">
          {['⏱', '📊', '📋'].map((icon) => (
            <button key={icon} type="button" className="pt-tool-btn" title="PT tool">{icon}</button>
          ))}
        </div>
      </div>

      <div className="pt-bottom">
        <div className="pt-tabs">
          {(['cli', 'config', 'desktop'] as const).map((tab) => (
            <button
              key={tab}
              type="button"
              className={`pt-tab ${bottomTab === tab ? 'active' : ''}`}
              onClick={() => setBottomTab(tab)}
            >
              {tab === 'cli' ? 'CLI' : tab === 'config' ? 'Config' : 'Desktop'}
            </button>
          ))}
          <span style={{ marginLeft: 'auto', padding: '4px 10px', fontSize: 10, color: '#555' }}>
            {Object.keys(nodes).length} devices
          </span>
        </div>
        {bottomTab === 'cli' && (
          <Terminal sendCommand={sendCommand} onRegisterHandler={onRegisterHandler} />
        )}
        {bottomTab === 'config' && (
          <div style={{ flex: 1, padding: 12, fontSize: 11, color: '#333', overflow: 'auto' }}>
            <strong>Config</strong> — select a device and use CLI tab for IOS configuration.
            <pre style={{ marginTop: 8, background: '#f5f5f5', padding: 8 }}>
{`! Site-to-Site VPN example
crypto isakmp policy 10
crypto isakmp key cisco123 address 203.0.113.2
crypto ipsec transform-set TS esp-aes esp-sha-hmac
crypto map VPNMAP 10 ipsec-isakmp
 set peer 203.0.113.2
 set transform-set TS
 match address VPN-TRAFFIC
interface Gi0/0
 crypto map VPNMAP`}
            </pre>
          </div>
        )}
        {bottomTab === 'desktop' && (
          <div style={{ flex: 1, padding: 12, fontSize: 11, color: '#333' }}>
            Desktop tab — host IP configuration (use CLI: <code>ip dhcp</code> or static on PC).
          </div>
        )}
        <div className="pt-sim-bar">
          <div className="pt-sim-toggle">
            <button type="button" className={simMode === 'realtime' ? 'active' : ''} onClick={() => setSimMode('realtime')}>Realtime</button>
            <button type="button" className={simMode === 'simulation' ? 'active' : ''} onClick={() => setSimMode('simulation')}>Simulation</button>
          </div>
        </div>
      </div>
    </div>
  );
};
