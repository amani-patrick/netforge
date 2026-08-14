import React, { useCallback, useState, useRef } from 'react';
import { useTopologyStore, NodeType } from '../store/useTopologyStore';
import { DevicePalette } from './DevicePalette';
import { TopologyCanvas } from './TopologyCanvas';
import { Terminal } from './Terminal';
import { InspectPanel } from './InspectPanel';
import { DeviceCategory, DeviceModel } from '../assets/deviceCatalog';
import type { EngineEventHandler } from '../hooks/useWebSocket';
import { useKeyboardShortcuts, useDeleteSelection } from '../hooks/useKeyboardShortcuts';

interface PacketTracerAppProps {
  sendCommand: (type: string, payload: unknown) => void;
  onRegisterHandler?: (handler: EngineEventHandler) => void;
}

function buildRouterInterfaces(routerIndex: number) {
  return [
    {
      port_id: 'GigabitEthernet0/0',
      ip: `192.168.${routerIndex}.1`,
      mask: '255.255.255.0',
      mac: `00:1A:2B:3C:${routerIndex.toString(16).padStart(2, '0')}:01`,
    },
    {
      port_id: 'GigabitEthernet0/1',
      ip: `10.0.${routerIndex}.1`,
      mask: '255.255.255.0',
      mac: `00:1A:2B:3C:${routerIndex.toString(16).padStart(2, '0')}:02`,
    },
  ];
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
    case 'CELLULAR_GW': return 'CELLULAR_GW';
    case 'MOBILE_UE': return 'MOBILE_UE';
    case 'CALL_MANAGER': return 'CALL_MANAGER';
    case 'COPPER':
    case 'FIBER':
      return null;
    default: return 'ROUTER';
  }
}

export const PacketTracerApp: React.FC<PacketTracerAppProps> = ({ sendCommand, onRegisterHandler }) => {
  const [category, setCategory] = useState<DeviceCategory>('routers');
  const [fileMenuOpen, setFileMenuOpen] = useState(false);
  const fileMenuRef = useRef<HTMLDivElement>(null);

  const {
    linkMode, setLinkMode, activeTool, setActiveTool,
    simMode, setSimMode, bottomTab, setBottomTab,
    nodes, setPendingPlacement, inspectPanelOpen, setInspectPanelOpen,
    clearTopology,
  } = useTopologyStore();

  // Keyboard: Delete/Backspace removes selected node or link.
  const handleDelete = useDeleteSelection(sendCommand);

  // Keyboard: Escape cancels link mode, pending placement, or resets to select tool.
  const handleEscape = useCallback(() => {
    setLinkMode(false);
    setPendingPlacement(null);
    setActiveTool('select');
    setFileMenuOpen(false);
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
      sendCommand('ADD_AP', { id: node.id, ssid: 'NetForge-WiFi', security: 'WPA2', password: 'cisco123' });

    } else if (type === 'PHONE') {
      sendCommand('ADD_VOIP_PHONE', { id: node.id });

    } else if (type === 'CELLULAR_GW') {
      sendCommand('ADD_CELLULAR_GATEWAY', { id: node.id });

    } else if (type === 'MOBILE_UE') {
      sendCommand('ADD_MOBILE_UE', { id: node.id });

    } else if (type === 'CALL_MANAGER') {
      sendCommand('ADD_CALL_MANAGER', {
        id: node.id,
        ip: `10.100.0.${Object.values(state.nodes).filter((n) => n.type === 'CALL_MANAGER').length + 1}`,
      });
    }

    return node;
  }, [sendCommand]);

  const handleDevicePick = (model: DeviceModel, label: string) => {
    if (model === 'COPPER' || model === 'FIBER') {
      setLinkMode(true, model === 'COPPER' ? 'copper' : 'fiber');
      return;
    }
    const type = mapModelToNodeType(model);
    if (!type) return;
    const cx = 200 + Math.random() * 400;
    const cy = 100 + Math.random() * 250;
    spawnDevice(type, label, cx, cy);
  };

  const loadVPNLab = () => {
    sendCommand('LOAD_VPN_LAB', {});
    const hq = spawnDevice('ROUTER', 'HQ', 260, 200);
    const branch = spawnDevice('ROUTER', 'Branch', 520, 200);
    spawnDevice('PC', 'HQ-PC', 260, 340);
    spawnDevice('PC', 'Branch-PC', 520, 340);
    spawnDevice('CLOUD', 'Internet', 390, 80);
    if (hq && branch) {
      const state = useTopologyStore.getState();
      const link = state.connectNodes(hq.id, 'GigabitEthernet0/0', branch.id, 'GigabitEthernet0/0', 'fiber');
      sendCommand('CONNECT_LINK', {
        id: link.id,
        source_node_id: hq.id,
        source_port_id: 'GigabitEthernet0/0',
        target_node_id: branch.id,
        target_port_id: 'GigabitEthernet0/0',
        bandwidth: 10_000_000,
      });
    }
    setFileMenuOpen(false);
  };

  const handleSaveTopology = () => {
    sendCommand('SAVE_TOPOLOGY', { path: 'topology.json' });
    setFileMenuOpen(false);
  };

  const handleLoadTopology = () => {
    sendCommand('LOAD_TOPOLOGY', { path: 'topology.json' });
    setFileMenuOpen(false);
  };

  const handleExportTopology = () => {
    sendCommand('EXPORT_TOPOLOGY', {});
    setFileMenuOpen(false);
  };

  const handleListDevices = () => {
    sendCommand('LIST_DEVICES', {});
    setFileMenuOpen(false);
  };

  const handleClearTopology = () => {
    clearTopology();
    setFileMenuOpen(false);
  };

  const handleEvaluateActivity = () => {
    sendCommand('EVALUATE_ACTIVITY', {});
    setFileMenuOpen(false);
  };

  const tools: { id: typeof activeTool; icon: string; title: string }[] = [
    { id: 'select', icon: '↖', title: 'Select & Move (S)' },
    { id: 'inspect', icon: '🔍', title: 'Inspect Device (I)' },
    { id: 'delete', icon: '✕', title: 'Delete (Del)' },
    { id: 'note', icon: '📝', title: 'Place Note (N)' },
    { id: 'move', icon: '✥', title: 'Pan Canvas (M)' },
  ];

  return (
    <div className="pt-app">
      {/* Menu Bar */}
      <div className="pt-menubar">
        <span className="pt-title">NetForge</span>

        {/* File dropdown */}
        <div className="pt-menu-item" ref={fileMenuRef} style={{ position: 'relative' }}>
          <span
            className="pt-menu-label"
            onClick={() => setFileMenuOpen(!fileMenuOpen)}
          >File</span>
          {fileMenuOpen && (
            <div className="pt-dropdown">
              <button type="button" onClick={loadVPNLab}>🔐 Load VPN Lab</button>
              <hr />
              <button type="button" onClick={handleSaveTopology}>💾 Save Topology</button>
              <button type="button" onClick={handleLoadTopology}>📂 Load Topology</button>
              <button type="button" onClick={handleExportTopology}>📤 Export JSON</button>
              <hr />
              <button type="button" onClick={handleListDevices}>📋 List Devices</button>
              <button type="button" onClick={handleEvaluateActivity}>🎯 Evaluate Activity</button>
              <hr />
              <button type="button" onClick={handleClearTopology} style={{ color: '#c62828' }}>🗑 Clear Workspace</button>
            </div>
          )}
        </div>

        <span className="pt-menu-label">Edit</span>
        <span className="pt-menu-label">View</span>
        <span className="pt-menu-label">Options</span>
        <span className="pt-menu-label">Help</span>
        <span style={{ marginLeft: 'auto', fontSize: 10, opacity: 0.8 }}>
          Logical | Physical
        </span>
      </div>

      {/* Toolbar */}
      <div className="pt-toolbar">
        {tools.map((t) => (
          <button
            key={t.id}
            type="button"
            className={`pt-tool-btn ${activeTool === t.id ? 'active' : ''}`}
            title={t.title}
            onClick={() => {
              setActiveTool(t.id);
              if (t.id === 'select') setLinkMode(false);
              if (t.id === 'inspect') setInspectPanelOpen(true);
            }}
          >
            {t.icon}
          </button>
        ))}
        <span style={{ marginLeft: 12, color: '#555' }}>|</span>

        <button
          type="button"
          className={`pt-tool-btn ${linkMode ? 'active' : ''}`}
          title="Connections (C)"
          onClick={() => setLinkMode(!linkMode)}
          style={{ width: 'auto', padding: '0 8px', fontSize: 11 }}
        >
          🔗 Connect
        </button>

        <button
          type="button"
          className={`pt-tool-btn ${inspectPanelOpen ? 'active' : ''}`}
          title="Inspect Panel"
          onClick={() => setInspectPanelOpen(!inspectPanelOpen)}
          style={{ width: 'auto', padding: '0 8px', fontSize: 11 }}
        >
          📊 Inspect
        </button>

        <span style={{ marginLeft: 'auto', color: '#777', fontSize: 10 }}>
          {Object.keys(nodes).length} devices
        </span>
      </div>

      {/* Main workspace */}
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

        {/* Inspect Panel — slides in on the right */}
        {inspectPanelOpen && (
          <InspectPanel sendCommand={sendCommand} onRegisterHandler={onRegisterHandler} />
        )}
      </div>

      {/* Bottom panel */}
      <div className="pt-bottom">
        <div className="pt-tabs">
          {(['cli', 'config', 'desktop'] as const).map((tab) => (
            <button
              key={tab}
              type="button"
              className={`pt-tab ${bottomTab === tab ? 'active' : ''}`}
              onClick={() => setBottomTab(tab)}
            >
              {tab === 'cli' ? '⌨ CLI' : tab === 'config' ? '⚙ Config' : '🖥 Desktop'}
            </button>
          ))}
        </div>

        {bottomTab === 'cli' && (
          <Terminal sendCommand={sendCommand} onRegisterHandler={onRegisterHandler} />
        )}

        {bottomTab === 'config' && (
          <div style={{ flex: 1, padding: 12, fontSize: 11, color: '#333', overflow: 'auto', background: '#fafafa' }}>
            <strong>Config Panel</strong> — select a device on the canvas, then use the CLI or Inspect panel.
            <br /><br />
            <strong>Quick IOS commands:</strong>
            <pre style={{ background: '#f0f0f0', padding: 10, borderRadius: 4, marginTop: 6, fontSize: 10 }}>
{`enable
configure terminal

! OSPF
router ospf 1
 network 10.0.0.0 0.0.0.255 area 0

! VLANs
vlan 10
 name SALES
interface Fa0/1
 switchport access vlan 10

! NAT Overload
interface Gi0/0
 ip nat inside
interface Gi0/1
 ip nat outside
 ip nat outside source overload

! HSRP
interface Gi0/0
 standby 1 ip 10.0.0.254
 standby 1 priority 110

! IPsec VPN
crypto isakmp policy 10
crypto isakmp key cisco123 address 203.0.113.2
crypto ipsec transform-set TS esp-aes esp-sha-hmac
crypto map VPNMAP 10 ipsec-isakmp
 set peer 203.0.113.2
 set transform-set TS
interface Gi0/0
 crypto map VPNMAP`}
            </pre>
          </div>
        )}

        {bottomTab === 'desktop' && (
          <div style={{ flex: 1, padding: 12, fontSize: 11, color: '#333', background: '#fafafa' }}>
            <strong>Desktop</strong> — quick actions for selected host:
            <br /><br />
            {Object.keys(nodes).filter((id) => {
              const n = nodes[id];
              return n.type === 'PC' || n.type === 'SERVER' || n.type === 'MOBILE_UE';
            }).length === 0
              ? <span style={{ color: '#888' }}>No hosts in topology. Add a PC or Mobile UE.</span>
              : Object.values(nodes)
                  .filter((n) => n.type === 'PC' || n.type === 'SERVER' || n.type === 'MOBILE_UE')
                  .map((n) => (
                    <div key={n.id} style={{ marginBottom: 10, padding: 8, background: '#fff', border: '1px solid #ddd', borderRadius: 4 }}>
                      <strong>{n.name}</strong> <span style={{ color: '#888', fontSize: 10 }}>({n.type})</span>
                      <div style={{ marginTop: 6, display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                        <button
                          type="button"
                          className="inspect-btn"
                          onClick={() => sendCommand('HOST_DHCP', { host_id: n.id })}
                        >DHCP Request</button>
                        <button
                          type="button"
                          className="inspect-btn"
                          onClick={() => sendCommand('SHOW_IP_ROUTE', { router_id: n.id })}
                        >Show Routes</button>
                        {n.type === 'MOBILE_UE' && (
                          <button
                            type="button"
                            className="inspect-btn"
                            onClick={() => sendCommand('ATTACH_5G_NR', {
                              ue_id: n.id, gateway_id: 'gw1',
                              ip: `172.16.0.${Math.floor(Math.random() * 200) + 10}`,
                              band: 'n78', arfcn: 632628,
                            })}
                          >Attach 5G NR</button>
                        )}
                      </div>
                    </div>
                  ))
            }
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
