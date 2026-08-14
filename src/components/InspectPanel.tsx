import React, { useState, useCallback, useEffect } from 'react';
import { useTopologyStore, NodeType } from '../store/useTopologyStore';
import type { EngineEventHandler } from '../hooks/useWebSocket';

interface InspectPanelProps {
  sendCommand: (type: string, payload: unknown) => void;
  onRegisterHandler?: (handler: EngineEventHandler) => void;
}

interface RouteRow { network: string; protocol: string; next_hop?: string; interface?: string; metric?: number }
interface OspfNeighbor { neighbor_id: string; interface: string; state: string }
interface VlanRow { id: number; name: string }
interface CallRow { call_id: string; from_phone: string; to_extension: string; state: string }
interface EventRow { category: string; node_id: string; port_id: string; message: string; sim_time?: number }
interface CaptureRow { frame_id?: string; src_mac?: string; dst_mac?: string; type?: string }

type Section = 'device' | 'routes' | 'ospf' | 'vlan' | 'hsrp' | 'crypto' | 'qos' | 'calls' | 'events' | 'capture';

const SECTION_LABELS: Record<Section, string> = {
  device: '📟 Device Info',
  routes: '🗺 IP Routes',
  ospf: '🔄 OSPF Neighbors',
  vlan: '🏷 VLANs',
  hsrp: '🛡 HSRP',
  crypto: '🔐 Crypto SA',
  qos: '⚡ QoS Policy',
  calls: '📞 VoIP Calls',
  events: '📋 Event Log',
  capture: '📦 Port Capture',
};

const SECTIONS_FOR_TYPE: Record<string, Section[]> = {
  ROUTER: ['device', 'routes', 'ospf', 'hsrp', 'crypto', 'qos', 'events', 'capture'],
  SWITCH: ['device', 'vlan', 'events', 'capture'],
  PC: ['device', 'events', 'capture'],
  SERVER: ['device', 'events', 'capture'],
  ASA: ['device', 'routes', 'events'],
  AP: ['device', 'events'],
  PHONE: ['device', 'calls', 'events'],
  CLOUD: ['device', 'events'],
  CELLULAR_GW: ['device', 'routes', 'events'],
  MOBILE_UE: ['device', 'events'],
  CALL_MANAGER: ['device', 'calls', 'events'],
};

export const InspectPanel: React.FC<InspectPanelProps> = ({ sendCommand, onRegisterHandler }) => {
  const { nodes, selectedNodeId, inspectPanelOpen, setInspectPanelOpen } = useTopologyStore();
  const node = selectedNodeId ? nodes[selectedNodeId] : null;

  const [activeSection, setActiveSection] = useState<Section>('device');
  const [routes, setRoutes] = useState<RouteRow[]>([]);
  const [ospfNeighbors, setOspfNeighbors] = useState<OspfNeighbor[]>([]);
  const [vlans, setVlans] = useState<VlanRow[]>([]);
  const [hsrpStatus, setHsrpStatus] = useState<string>('');
  const [cryptoSA, setCryptoSA] = useState<string>('');
  const [qosPolicy, setQosPolicy] = useState<string>('');
  const [calls, setCalls] = useState<CallRow[]>([]);
  const [events, setEvents] = useState<EventRow[]>([]);
  const [captures, setCaptures] = useState<CaptureRow[]>([]);
  const [capturePort, setCapturePort] = useState<string>('GigabitEthernet0/0');
  const [loading, setLoading] = useState(false);

  // Ping dialog
  const [pingTarget, setPingTarget] = useState('');
  const [pingResult, setPingResult] = useState<string | null>(null);

  // SIP call dialog
  const [sipCallee, setSipCallee] = useState('');
  const [sipResult, setSipResult] = useState<string | null>(null);

  // Activity assessment
  const [activityResults, setActivityResults] = useState<{ id: string; passed: boolean; message: string }[]>([]);

  const handleEngineEvent = useCallback<EngineEventHandler>((event, data) => {
    const d = data as Record<string, unknown>;
    switch (event) {
      case 'ROUTE_TABLE':
        setRoutes((d.routes as RouteRow[]) ?? []);
        setLoading(false);
        break;
      case 'OSPF_NEIGHBORS':
        setOspfNeighbors((d.neighbors as OspfNeighbor[]) ?? []);
        setLoading(false);
        break;
      case 'VLAN_TABLE':
        setVlans((d as unknown as VlanRow[]) ?? []);
        setLoading(false);
        break;
      case 'HSRP_STATUS':
        setHsrpStatus(JSON.stringify(d, null, 2));
        setLoading(false);
        break;
      case 'CRYPTO_SA':
        setCryptoSA(JSON.stringify(d, null, 2));
        setLoading(false);
        break;
      case 'QOS_POLICY':
        setQosPolicy(JSON.stringify(d, null, 2));
        setLoading(false);
        break;
      case 'VOIP_CALLS':
        setCalls((data as CallRow[]) ?? []);
        setLoading(false);
        break;
      case 'EVENT_LOG':
        setEvents((data as EventRow[]) ?? []);
        setLoading(false);
        break;
      case 'PORT_CAPTURE':
        setCaptures((data as CaptureRow[]) ?? []);
        setLoading(false);
        break;
      case 'PING_RESULT':
        if (d.source_id === selectedNodeId) {
          setPingResult(d.success
            ? `✅ Success — RTT ${d.rtt_ms ?? '?'}ms`
            : `❌ Timeout — ${d.message ?? 'no reply'}`
          );
        }
        break;
      case 'SIP_CALL_STARTED':
        setSipResult(`✅ Call started: ${JSON.stringify(d.call_id ?? d)}`);
        break;
      case 'SCCP_REGISTERED':
        setSipResult('✅ Phone registered with Call Manager');
        break;
      case 'ACTIVITY_RESULTS':
        setActivityResults((data as { id: string; passed: boolean; message: string }[]) ?? []);
        break;
    }
  }, [selectedNodeId]);

  useEffect(() => {
    onRegisterHandler?.(handleEngineEvent);
  }, [handleEngineEvent, onRegisterHandler]);

  // Auto-load data when section changes
  useEffect(() => {
    if (!node) return;
    setLoading(true);
    switch (activeSection) {
      case 'routes':
        sendCommand('SHOW_IP_ROUTE', { router_id: node.id });
        break;
      case 'ospf':
        sendCommand('SHOW_OSPF_NEIGHBOR', { router_id: node.id });
        break;
      case 'vlan':
        sendCommand('SHOW_VLAN', { switch_id: node.id });
        break;
      case 'hsrp':
        sendCommand('SHOW_HSRP', { router_id: node.id });
        break;
      case 'crypto':
        sendCommand('SHOW_CRYPTO_SA', { router_id: node.id });
        break;
      case 'qos':
        sendCommand('SHOW_QOS', { router_id: node.id });
        break;
      case 'calls':
        sendCommand('SHOW_CALLS', {});
        break;
      case 'events':
        sendCommand('GET_EVENT_LOG', { limit: 50, category: '' });
        break;
      default:
        setLoading(false);
    }
  }, [activeSection, node, sendCommand]);

  // Reset on node change
  useEffect(() => {
    setActiveSection('device');
    setRoutes([]);
    setOspfNeighbors([]);
    setVlans([]);
    setPingResult(null);
    setSipResult(null);
    setActivityResults([]);
  }, [selectedNodeId]);

  if (!inspectPanelOpen) return null;

  const sections: Section[] = node
    ? (SECTIONS_FOR_TYPE[node.type as NodeType] ?? ['device', 'events'])
    : ['events'];

  return (
    <div className="inspect-panel">
      <div className="inspect-header">
        <span className="inspect-title">
          {node ? `${node.name} — ${node.type}` : 'Inspect Panel'}
        </span>
        <button
          type="button"
          className="inspect-close-btn"
          onClick={() => setInspectPanelOpen(false)}
          title="Close"
        >✕</button>
      </div>

      {/* Section tabs */}
      <div className="inspect-tabs">
        {sections.map((s) => (
          <button
            key={s}
            type="button"
            className={`inspect-tab ${activeSection === s ? 'active' : ''}`}
            onClick={() => setActiveSection(s)}
          >
            {SECTION_LABELS[s]}
          </button>
        ))}
      </div>

      <div className="inspect-body">
        {loading && <div className="inspect-loading">Loading…</div>}

        {/* ─── Device Info ─── */}
        {activeSection === 'device' && node && (
          <div className="inspect-section">
            <table className="inspect-table">
              <tbody>
                <tr><td>ID</td><td><code>{node.id}</code></td></tr>
                <tr><td>Type</td><td>{node.type}</td></tr>
                <tr><td>Model</td><td>{node.modelLabel}</td></tr>
                <tr><td>MAC</td><td><code>{node.macAddress}</code></td></tr>
                {node.ipAddress && <tr><td>IP</td><td><code>{node.ipAddress}</code></td></tr>}
              </tbody>
            </table>

            {/* Ping */}
            <div className="inspect-action-group">
              <div className="inspect-action-title">🏓 Ping</div>
              <div className="inspect-row">
                <input
                  className="inspect-input"
                  placeholder="Destination IP"
                  value={pingTarget}
                  onChange={(e) => setPingTarget(e.target.value)}
                />
                <button
                  type="button"
                  className="inspect-btn primary"
                  onClick={() => {
                    if (!pingTarget) return;
                    setPingResult(null);
                    sendCommand('TRIGGER_PING', {
                      src_id: node.id,
                      dst_ip: pingTarget,
                      request_id: `inspect_${Date.now()}`,
                    });
                  }}
                >Ping</button>
              </div>
              {pingResult && <div className="inspect-result">{pingResult}</div>}
            </div>

            {/* Traceroute */}
            <div className="inspect-action-group">
              <div className="inspect-action-title">🛤 Traceroute</div>
              <div className="inspect-row">
                <input
                  className="inspect-input"
                  placeholder="Destination IP"
                  value={pingTarget}
                  onChange={(e) => setPingTarget(e.target.value)}
                />
                <button
                  type="button"
                  className="inspect-btn"
                  onClick={() => {
                    if (!pingTarget) return;
                    sendCommand('TRACEROUTE', { source_id: node.id, dest_ip: pingTarget });
                  }}
                >Trace</button>
              </div>
            </div>

            {/* VoIP phone controls */}
            {(node.type === 'PHONE') && (
              <div className="inspect-action-group">
                <div className="inspect-action-title">📞 VoIP Controls</div>
                <div className="inspect-row">
                  <input
                    className="inspect-input"
                    placeholder="Callee extension (e.g. 1001)"
                    value={sipCallee}
                    onChange={(e) => setSipCallee(e.target.value)}
                  />
                  <button
                    type="button"
                    className="inspect-btn primary"
                    onClick={() => {
                      if (!sipCallee) return;
                      setSipResult(null);
                      sendCommand('SIP_CALL', { phone_id: node.id, callee: sipCallee });
                    }}
                  >Call</button>
                </div>
                <button
                  type="button"
                  className="inspect-btn"
                  style={{ marginTop: 6, width: '100%' }}
                  onClick={() => sendCommand('SCCP_REGISTER', { phone_id: node.id, cm_id: 'cm1' })}
                >SCCP Register</button>
                {sipResult && <div className="inspect-result">{sipResult}</div>}
              </div>
            )}

            {/* Cellular / 5G controls */}
            {(node.type === 'CELLULAR_GW') && (
              <div className="inspect-action-group">
                <div className="inspect-action-title">📡 5G Status</div>
                <button
                  type="button"
                  className="inspect-btn"
                  style={{ width: '100%' }}
                  onClick={() => sendCommand('SHOW_5G_STATUS', { gateway_id: node.id })}
                >Show 5G Status</button>
              </div>
            )}

            {/* Activity Assessment */}
            <div className="inspect-action-group">
              <div className="inspect-action-title">🎯 Assessment</div>
              <button
                type="button"
                className="inspect-btn"
                style={{ width: '100%' }}
                onClick={() => {
                  setActivityResults([]);
                  sendCommand('EVALUATE_ACTIVITY', {});
                }}
              >Evaluate Activity Goals</button>
              {activityResults.length > 0 && (
                <div className="inspect-assess-results">
                  {activityResults.map((r) => (
                    <div key={r.id} className={`inspect-assess-row ${r.passed ? 'pass' : 'fail'}`}>
                      {r.passed ? '✅' : '❌'} <strong>{r.id}</strong>: {r.message}
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Write memory */}
            <div className="inspect-action-group">
              <button
                type="button"
                className="inspect-btn"
                style={{ width: '100%' }}
                onClick={() => sendCommand('WRITE_MEMORY', { device_id: node.id })}
              >💾 Write Memory (copy run start)</button>
            </div>
          </div>
        )}

        {/* ─── IP Routes ─── */}
        {activeSection === 'routes' && !loading && (
          <div className="inspect-section">
            {routes.length === 0
              ? <div className="inspect-empty">No routes. Configure OSPF, RIP or static routes via CLI.</div>
              : (
                <table className="inspect-table full-width">
                  <thead><tr><th>Network</th><th>Protocol</th><th>Next Hop</th><th>Interface</th><th>Metric</th></tr></thead>
                  <tbody>
                    {routes.map((r, i) => (
                      <tr key={i}>
                        <td><code>{r.network}</code></td>
                        <td><span className={`badge proto-${r.protocol}`}>{r.protocol}</span></td>
                        <td><code>{r.next_hop ?? '—'}</code></td>
                        <td>{r.interface ?? '—'}</td>
                        <td>{r.metric ?? '—'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )
            }
          </div>
        )}

        {/* ─── OSPF Neighbors ─── */}
        {activeSection === 'ospf' && !loading && (
          <div className="inspect-section">
            {ospfNeighbors.length === 0
              ? <div className="inspect-empty">No OSPF neighbors. Run: router ospf 1 → network … area 0</div>
              : (
                <table className="inspect-table full-width">
                  <thead><tr><th>Neighbor ID</th><th>Interface</th><th>State</th></tr></thead>
                  <tbody>
                    {ospfNeighbors.map((n, i) => (
                      <tr key={i}>
                        <td><code>{n.neighbor_id}</code></td>
                        <td>{n.interface}</td>
                        <td><span className={`badge state-${n.state}`}>{n.state}</span></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )
            }
          </div>
        )}

        {/* ─── VLAN Table ─── */}
        {activeSection === 'vlan' && !loading && (
          <div className="inspect-section">
            {vlans.length === 0
              ? <div className="inspect-empty">No VLANs. Run: vlan 10 → name SALES</div>
              : (
                <table className="inspect-table full-width">
                  <thead><tr><th>VLAN ID</th><th>Name</th></tr></thead>
                  <tbody>
                    {vlans.map((v, i) => (
                      <tr key={i}>
                        <td><strong>{v.id}</strong></td>
                        <td>{v.name}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )
            }
          </div>
        )}

        {/* ─── HSRP ─── */}
        {activeSection === 'hsrp' && !loading && (
          <div className="inspect-section">
            {!hsrpStatus || hsrpStatus === 'null' || hsrpStatus === '[]'
              ? <div className="inspect-empty">No HSRP configured. Run: standby 1 ip …</div>
              : <pre className="inspect-pre">{hsrpStatus}</pre>
            }
          </div>
        )}

        {/* ─── Crypto SA ─── */}
        {activeSection === 'crypto' && !loading && (
          <div className="inspect-section">
            {!cryptoSA || cryptoSA === 'null' || cryptoSA === '[]'
              ? <div className="inspect-empty">No IPsec SA. Configure crypto map and run NEGOTIATE_IKE.</div>
              : <pre className="inspect-pre">{cryptoSA}</pre>
            }
            <button
              type="button"
              className="inspect-btn primary"
              style={{ marginTop: 8, width: '100%' }}
              onClick={() => sendCommand('LOAD_VPN_LAB', {})}
            >Load VPN Lab Preset</button>
          </div>
        )}

        {/* ─── QoS Policy ─── */}
        {activeSection === 'qos' && !loading && (
          <div className="inspect-section">
            {!qosPolicy || qosPolicy === 'null' || qosPolicy === '[]'
              ? <div className="inspect-empty">No QoS configured. Use: class-map → policy-map → service-policy</div>
              : <pre className="inspect-pre">{qosPolicy}</pre>
            }
          </div>
        )}

        {/* ─── VoIP Calls ─── */}
        {activeSection === 'calls' && !loading && (
          <div className="inspect-section">
            {calls.length === 0
              ? <div className="inspect-empty">No active calls. Use SIP_CALL or SCCP_REGISTER.</div>
              : (
                <table className="inspect-table full-width">
                  <thead><tr><th>Call ID</th><th>From</th><th>To</th><th>State</th></tr></thead>
                  <tbody>
                    {calls.map((c, i) => (
                      <tr key={i}>
                        <td><code>{c.call_id}</code></td>
                        <td>{c.from_phone}</td>
                        <td>{c.to_extension}</td>
                        <td><span className="badge">{c.state}</span></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )
            }
          </div>
        )}

        {/* ─── Event Log ─── */}
        {activeSection === 'events' && !loading && (
          <div className="inspect-section">
            <div className="inspect-row" style={{ marginBottom: 8 }}>
              <button
                type="button"
                className="inspect-btn"
                onClick={() => {
                  setLoading(true);
                  sendCommand('GET_EVENT_LOG', { limit: 50, category: '' });
                }}
              >↻ Refresh</button>
              <button
                type="button"
                className="inspect-btn danger"
                onClick={() => {
                  sendCommand('CLEAR_EVENT_LOG', {});
                  setEvents([]);
                }}
              >🗑 Clear</button>
            </div>
            {events.length === 0
              ? <div className="inspect-empty">No events. Activity is logged here as you configure devices.</div>
              : (
                <div className="inspect-event-log">
                  {events.map((e, i) => (
                    <div key={i} className={`inspect-event event-${e.category}`}>
                      <span className="event-tag">[{e.category}]</span>
                      <span className="event-node">{e.node_id}{e.port_id ? ':' + e.port_id : ''}</span>
                      <span className="event-msg">{e.message}</span>
                    </div>
                  ))}
                </div>
              )
            }
          </div>
        )}

        {/* ─── Port Capture ─── */}
        {activeSection === 'capture' && !loading && (
          <div className="inspect-section">
            <div className="inspect-row" style={{ marginBottom: 8 }}>
              <input
                className="inspect-input"
                placeholder="Port (e.g. GigabitEthernet0/0)"
                value={capturePort}
                onChange={(e) => setCapturePort(e.target.value)}
              />
              <button
                type="button"
                className="inspect-btn primary"
                onClick={() => {
                  if (!node) return;
                  setLoading(true);
                  sendCommand('GET_PORT_CAPTURE', { node_id: node.id, port_id: capturePort, limit: 20 });
                }}
              >Capture</button>
            </div>
            {captures.length === 0
              ? <div className="inspect-empty">No captured frames. Click Capture to fetch.</div>
              : (
                <table className="inspect-table full-width">
                  <thead><tr><th>Src MAC</th><th>Dst MAC</th><th>Type</th></tr></thead>
                  <tbody>
                    {captures.map((c, i) => (
                      <tr key={i}>
                        <td><code>{c.src_mac ?? '—'}</code></td>
                        <td><code>{c.dst_mac ?? '—'}</code></td>
                        <td>{c.type ?? '—'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )
            }
          </div>
        )}
      </div>
    </div>
  );
};
