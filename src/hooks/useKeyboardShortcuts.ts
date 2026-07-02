import { useEffect } from 'react';
import { useTopologyStore } from '../store/useTopologyStore';

export function useKeyboardShortcuts(onDelete: () => void, onEscape: () => void) {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;

      if (e.key === 'Delete' || e.key === 'Backspace') {
        e.preventDefault();
        onDelete();
      }
      if (e.key === 'Escape') {
        onEscape();
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [onDelete, onEscape]);
}

export function useDeleteSelection(sendCommand: (type: string, payload: unknown) => void) {
  const { selectedNodeId, selectedLinkId, removeNode, removeLink } = useTopologyStore();

  return () => {
    if (selectedLinkId) {
      removeLink(selectedLinkId);
      sendCommand('REMOVE_LINK', { id: selectedLinkId });
      return;
    }
    if (selectedNodeId) {
      removeNode(selectedNodeId);
      sendCommand('REMOVE_DEVICE', { id: selectedNodeId });
    }
  };
}
