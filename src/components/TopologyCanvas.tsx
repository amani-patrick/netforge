import React, { useState, useRef, useCallback } from 'react';
import { useTopologyStore, NetworkNode, NodeType } from '../store/useTopologyStore';
import { DeviceIcon } from '../assets/DeviceIcons';
import type { DeviceModel } from '../assets/deviceCatalog';

interface TopologyCanvasProps {
  sendCommand: (type: string, payload: unknown) => void;
  onSpawnDevice: (type: NodeType, label: string, x: number, y: number) => void;
}

function defaultPort(type: NodeType, index: number): string {
  if (type === 'ROUTER') return index === 0 ? 'GigabitEthernet0/0' : 'GigabitEthernet0/1';
  if (type === 'SWITCH') return `Fa0/${index + 1}`;
  if (type === 'ASA') return 'GigabitEthernet0/0';
  return 'Eth0';
}

function modelFromType(type: NodeType): DeviceModel {
  if (type === 'SERVER') return 'SERVER';
  if (type === 'ASA') return 'ASA';
  if (type === 'AP') return 'AP';
  if (type === 'PHONE') return 'PHONE';
  if (type === 'CLOUD') return 'CLOUD';
  if (type === 'SWITCH') return 'SWITCH';
  if (type === 'PC') return 'PC';
  return 'ROUTER';
}

export const TopologyCanvas: React.FC<TopologyCanvasProps> = ({ sendCommand, onSpawnDevice }) => {
  const {
    nodes, links, updateNodePosition, selectedNodeId, selectNode,
    linkMode, linkSourceId, setLinkSourceId, connectNodes, setLinkMode,
    activeTool, removeNode,
  } = useTopologyStore();
  const [draggingNodeId, setDraggingNodeId] = useState<string | null>(null);
  const workspaceRef = useRef<HTMLDivElement | null>(null);

  const handleWorkspaceClick = (e: React.MouseEvent) => {
    if (e.target !== workspaceRef.current) return;
    if (linkMode) return;
    selectNode(null);
  };

  const handleWorkspaceDoubleClick = (e: React.MouseEvent) => {
    if (!workspaceRef.current) return;
    const rect = workspaceRef.current.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;
    onSpawnDevice('ROUTER', '2911', x, y);
  };

  const completeLink = (targetId: string) => {
    if (!linkSourceId || linkSourceId === targetId) return;
    const source = nodes[linkSourceId];
    const target = nodes[targetId];
    const sourceLinks = links.filter((l) => l.sourceNodeId === linkSourceId || l.targetNodeId === linkSourceId);
    const targetLinks = links.filter((l) => l.sourceNodeId === targetId || l.targetNodeId === targetId);
    const sourcePort = defaultPort(source.type, sourceLinks.length);
    const targetPort = defaultPort(target.type, targetLinks.length);
    const link = connectNodes(linkSourceId, sourcePort, targetId, targetPort, 'copper');
    sendCommand('CONNECT_LINK', {
      id: link.id,
      source_node_id: linkSourceId,
      source_port_id: sourcePort,
      target_node_id: targetId,
      target_port_id: targetPort,
    });
    setLinkSourceId(null);
    setLinkMode(false);
    selectNode(targetId);
  };

  const handleNodeMouseDown = (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    if (activeTool === 'delete') {
      removeNode(id);
      sendCommand('REMOVE_DEVICE', { id });
      return;
    }
    if (linkMode || activeTool === 'select') {
      if (linkMode) {
        if (!linkSourceId) {
          setLinkSourceId(id);
          selectNode(id);
        } else {
          completeLink(id);
        }
        return;
      }
    }
    setDraggingNodeId(id);
    selectNode(id);
  };

  const handleMouseMove = (e: React.MouseEvent) => {
    if (!draggingNodeId || !workspaceRef.current || activeTool !== 'select') return;
    const rect = workspaceRef.current.getBoundingClientRect();
    updateNodePosition(draggingNodeId, e.clientX - rect.left, e.clientY - rect.top);
  };

  return (
    <div
      ref={workspaceRef}
      className="pt-workspace"
      onClick={handleWorkspaceClick}
      onDoubleClick={handleWorkspaceDoubleClick}
      onMouseMove={handleMouseMove}
      onMouseUp={() => setDraggingNodeId(null)}
      onMouseLeave={() => setDraggingNodeId(null)}
    >
      <svg style={{ position: 'absolute', inset: 0, width: '100%', height: '100%', pointerEvents: 'none' }}>
        {links.map((link) => {
          const source = nodes[link.sourceNodeId];
          const target = nodes[link.targetNodeId];
          if (!source || !target) return null;
          return (
            <line
              key={link.id}
              x1={source.x}
              y1={source.y}
              x2={target.x}
              y2={target.y}
              className={`pt-link-line ${link.cableType}`}
            />
          );
        })}
      </svg>

      {Object.values(nodes).map((node: NetworkNode) => (
        <div
          key={node.id}
          className={`pt-node ${selectedNodeId === node.id ? 'selected' : ''} ${linkSourceId === node.id ? 'link-source' : ''}`}
          style={{ left: node.x, top: node.y }}
          onMouseDown={(e) => handleNodeMouseDown(node.id, e)}
        >
          <div className="pt-node-box">
            <DeviceIcon model={modelFromType(node.type)} size={52} />
          </div>
          <div className="pt-node-label">{node.name}</div>
        </div>
      ))}
    </div>
  );
};
