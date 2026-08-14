import { create } from 'zustand';

export type NodeType = 'ROUTER' | 'SWITCH' | 'PC' | 'SERVER' | 'ASA' | 'AP' | 'PHONE' | 'CLOUD' | 'CELLULAR_GW' | 'MOBILE_UE' | 'CALL_MANAGER';
export type WorkspaceTool = 'select' | 'inspect' | 'delete' | 'note' | 'move';
export type SimMode = 'realtime' | 'simulation';

export interface NetworkNode {
  id: string;
  name: string;
  type: NodeType;
  modelLabel: string;
  x: number;
  y: number;
  ipAddress?: string;
  macAddress: string;
}

export interface NetworkLink {
  id: string;
  sourceNodeId: string;
  sourcePortId: string;
  targetNodeId: string;
  targetPortId: string;
  cableType: 'copper' | 'fiber';
}

export interface PendingPlacement {
  type: NodeType;
  label: string;
  model: string;
}

interface TopologyState {
  nodes: Record<string, NetworkNode>;
  links: NetworkLink[];
  selectedNodeId: string | null;
  selectedLinkId: string | null;
  linkMode: boolean;
  linkSourceId: string | null;
  pendingCableType: 'copper' | 'fiber' | null;
  pendingPlacement: PendingPlacement | null;
  activeTool: WorkspaceTool;
  simMode: SimMode;
  bottomTab: 'cli' | 'config' | 'desktop';
  statusMessage: string;
  inspectPanelOpen: boolean;

  addNode: (type: NodeType, label: string, x: number, y: number) => NetworkNode;
  updateNodePosition: (id: string, x: number, y: number) => void;
  connectNodes: (sourceId: string, sourcePort: string, targetId: string, targetPort: string, cableType?: 'copper' | 'fiber') => NetworkLink;
  selectNode: (id: string | null) => void;
  selectLink: (id: string | null) => void;
  clearSelection: () => void;
  setLinkMode: (on: boolean, cableType?: 'copper' | 'fiber') => void;
  setLinkSourceId: (id: string | null) => void;
  setPendingPlacement: (p: PendingPlacement | null) => void;
  setActiveTool: (tool: WorkspaceTool) => void;
  setSimMode: (mode: SimMode) => void;
  setBottomTab: (tab: 'cli' | 'config' | 'desktop') => void;
  setStatusMessage: (msg: string) => void;
  setInspectPanelOpen: (open: boolean) => void;
  removeNode: (id: string) => void;
  removeLink: (id: string) => void;
  clearTopology: () => void;
}

export const useTopologyStore = create<TopologyState>((set, get) => ({
  nodes: {},
  links: [],
  selectedNodeId: null,
  selectedLinkId: null,
  linkMode: false,
  linkSourceId: null,
  pendingCableType: null,
  pendingPlacement: null,
  activeTool: 'select',
  simMode: 'realtime',
  bottomTab: 'cli',
  statusMessage: 'Select tool active — click a device to select, drag to move. Press Del to delete.',
  inspectPanelOpen: false,

  addNode: (type, label, x, y) => {
    const id = `${type.toLowerCase()}_${Math.random().toString(36).substr(2, 9)}`;
    const count = Object.values(get().nodes).filter((n) => n.type === type).length + 1;
    const macAddress = Array.from({ length: 6 }, () =>
      Math.floor(Math.random() * 256).toString(16).padStart(2, '0').toUpperCase()
    ).join(':');

    const newNode: NetworkNode = {
      id,
      name: `${label.replace(/[^a-zA-Z0-9]/g, '')}${count}`,
      type,
      modelLabel: label,
      x,
      y,
      macAddress,
    };

    set((state) => ({
      nodes: { ...state.nodes, [id]: newNode },
      selectedNodeId: id,
      selectedLinkId: null,
      statusMessage: `Added ${newNode.name} — click to select, drag to move.`,
    }));
    return newNode;
  },

  updateNodePosition: (id, x, y) =>
    set((state) => {
      if (!state.nodes[id]) return {};
      return { nodes: { ...state.nodes, [id]: { ...state.nodes[id], x, y } } };
    }),

  connectNodes: (sourceNodeId, sourcePortId, targetNodeId, targetPortId, cableType = 'copper') => {
    const id = `link_${Date.now()}`;
    const newLink: NetworkLink = { id, sourceNodeId, sourcePortId, targetNodeId, targetPortId, cableType };
    set((state) => ({
      links: [...state.links, newLink],
      selectedLinkId: id,
      selectedNodeId: null,
      linkMode: false,
      linkSourceId: null,
      pendingCableType: null,
      statusMessage: `Cable connected (${cableType}). Click cable to select, Del to remove.`,
    }));
    return newLink;
  },

  selectNode: (id) => set({
    selectedNodeId: id,
    selectedLinkId: null,
    statusMessage: id
      ? `Selected device — drag to move, Del or toolbar to delete.`
      : 'Selection cleared.',
  }),

  selectLink: (id) => set({
    selectedLinkId: id,
    selectedNodeId: null,
    statusMessage: id
      ? `Selected cable — press Del or click Delete to remove.`
      : 'Selection cleared.',
  }),

  clearSelection: () => set({ selectedNodeId: null, selectedLinkId: null }),

  setLinkMode: (on, cableType) => set({
    linkMode: on,
    linkSourceId: null,
    pendingPlacement: null,
    pendingCableType: on ? (cableType ?? 'copper') : null,
    activeTool: on ? 'select' : get().activeTool,
    statusMessage: on
      ? `Cable mode (${cableType ?? 'copper'}) — click first device, then second. Esc to cancel.`
      : 'Cable mode cancelled.',
  }),

  setLinkSourceId: (id) => set({
    linkSourceId: id,
    statusMessage: id
      ? 'Click the second device to complete the cable.'
      : get().statusMessage,
  }),

  setPendingPlacement: (p) => set({
    pendingPlacement: p,
    linkMode: false,
    linkSourceId: null,
    statusMessage: p
      ? `Placing ${p.label} — click on workspace to place. Esc to cancel.`
      : get().statusMessage,
  }),

  setActiveTool: (tool) => set({
    activeTool: tool,
    pendingPlacement: null,
    statusMessage:
      tool === 'delete'
        ? 'Delete tool — click any device or cable to remove it.'
        : tool === 'select'
        ? 'Select tool — click to select, drag to move, Del to delete.'
        : `${tool} tool active.`,
  }),

  setSimMode: (mode) => set({ simMode: mode }),
  setBottomTab: (tab) => set({ bottomTab: tab }),
  setStatusMessage: (msg) => set({ statusMessage: msg }),
  setInspectPanelOpen: (open) => set({ inspectPanelOpen: open }),

  removeNode: (id) =>
    set((state) => {
      const { [id]: _, ...nodes } = state.nodes;
      return {
        nodes,
        links: state.links.filter((l) => l.sourceNodeId !== id && l.targetNodeId !== id),
        selectedNodeId: state.selectedNodeId === id ? null : state.selectedNodeId,
        statusMessage: 'Device deleted.',
      };
    }),

  removeLink: (id) =>
    set((state) => ({
      links: state.links.filter((l) => l.id !== id),
      selectedLinkId: state.selectedLinkId === id ? null : state.selectedLinkId,
      statusMessage: 'Cable deleted.',
    })),

  clearTopology: () => set({
    nodes: {},
    links: [],
    selectedNodeId: null,
    selectedLinkId: null,
    linkSourceId: null,
    linkMode: false,
    pendingPlacement: null,
    statusMessage: 'Workspace cleared.',
  }),
}));
