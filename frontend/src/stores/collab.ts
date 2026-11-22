import { create } from 'zustand';
import {
  type BoardMode,
  type BoardOp,
  type HostState,
  LIMITS,
  type Stroke,
  applyOp,
  mergeSnapshot,
  initialHostState,
} from '@/features/collab/protocol';

interface CollabState {
  /** As announced by the host (or our own, when we are the host). */
  host: HostState;
  strokes: Stroke[];
  setHost: (patch: Partial<HostState>) => void;
  apply: (by: string, op: BoardOp) => void;
  mergeSnapshot: (strokes: Stroke[]) => void;
  reset: () => void;
}

/** In-room collaboration state: recording flag and the shared whiteboard. */
export const useCollabStore = create<CollabState>()((set) => ({
  host: initialHostState,
  strokes: [],
  setHost: (patch) => set((s) => ({ host: { ...s.host, ...patch } })),
  apply: (by, op) => set((s) => ({ strokes: applyOp(s.strokes, by, op) })),
  mergeSnapshot: (snapshot) => set((s) => ({ strokes: mergeSnapshot(s.strokes, snapshot) })),
  reset: () => set({ host: initialHostState, strokes: [] }),
}));

const state = () => useCollabStore.getState();
const without = (list: string[], id: string) => list.filter((x) => x !== id);

/** Host-side whiteboard decisions; the sync hook broadcasts every change. */
export const boardHost = {
  open: (mode: BoardMode) => state().setHost({ board: mode, requests: [] }),
  close: () => state().setHost({ board: 'off', requests: [] }),
  setMode: (board: BoardMode) =>
    state().setHost({ board, requests: board === 'collab' ? state().host.requests : [] }),
  request: (id: string) => {
    const h = state().host;
    const skip = h.board !== 'collab' || h.requests.includes(id) || h.drawers.includes(id);
    if (!skip && h.requests.length < LIMITS.ids) state().setHost({ requests: [...h.requests, id] });
  },
  approve: (id: string) => {
    const h = state().host;
    const drawers = h.drawers.includes(id) ? h.drawers : [...h.drawers, id];
    state().setHost({ drawers: drawers.slice(-LIMITS.ids), requests: without(h.requests, id) });
  },
  deny: (id: string) => state().setHost({ requests: without(state().host.requests, id) }),
  revoke: (id: string) => state().setHost({ drawers: without(state().host.drawers, id) }),
};
