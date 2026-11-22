// Peer-to-peer collaboration protocol (LiveKit data messages). Every message
// is validated here; the sender's identity and role come from LiveKit, which
// takes them from the server-signed token, so they cannot be forged.

export type BoardMode = 'off' | 'solo' | 'collab';

/** State only the host may announce: recording and whiteboard permissions. */
export interface HostState {
  recording: boolean;
  board: BoardMode;
  drawers: string[]; // identities the host allowed to draw (collab mode)
  requests: string[]; // identities asking to draw
}

export const initialHostState: HostState = {
  recording: false,
  board: 'off',
  drawers: [],
  requests: [],
};

export const TOPIC = {
  host: 'lamma.host', // host → all: HostState
  guest: 'lamma.guest', // guest → host: hello, draw requests
  board: 'lamma.board', // drawer → all: stroke operations
  snapshot: 'lamma.board.snapshot', // host → one: all strokes (text stream)
} as const;

export const COLORS = ['#111827', '#dc2626', '#ea580c', '#16a34a', '#2563eb', '#9333ea'] as const;
export const WIDTHS = [2, 5, 10] as const; // thousandths of the board width

/** Bounds that keep memory and redraw cost small on every client. */
export const LIMITS = {
  segPoints: 512, // numbers per message
  strokePoints: 2_000, // numbers per stroke
  strokes: 1_500,
  strokesPerAuthor: 300, // one drawer cannot fill the board for everyone
  boardPoints: 200_000, // numbers on the whole board
  ids: 300,
  idLen: 64,
};

/** A freehand line; points are x,y pairs normalised to 0..1 of the board. */
export interface Stroke {
  id: string;
  by: string;
  color: string;
  width: number;
  points: number[];
}

export type BoardOp =
  | { t: 'seg'; id: string; color: string; width: number; points: number[] }
  | { t: 'undo'; id: string }
  | { t: 'clear' };

export type GuestMsg = { t: 'hello' } | { t: 'draw-request' } | { t: 'draw-cancel' };

const MODES: readonly BoardMode[] = ['off', 'solo', 'collab'];
const isObj = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null;
const isId = (v: unknown): v is string =>
  typeof v === 'string' && v.length > 0 && v.length <= LIMITS.idLen;
const isIdList = (v: unknown): v is string[] =>
  Array.isArray(v) && v.length <= LIMITS.ids && v.every(isId);
const isColor = (v: unknown): v is string => COLORS.some((c) => c === v);
const isWidth = (v: unknown): v is number => WIDTHS.some((w) => w === v);
const isPoints = (v: unknown, max: number): v is number[] =>
  Array.isArray(v) &&
  v.length % 2 === 0 &&
  v.length <= max &&
  v.every((n) => typeof n === 'number' && n >= 0 && n <= 1);

export function parseHostState(v: unknown): HostState | null {
  if (!isObj(v) || v.t !== 'state') return null;
  const { recording, board, drawers, requests } = v;
  const mode = MODES.find((m) => m === board);
  if (typeof recording !== 'boolean' || !mode || !isIdList(drawers) || !isIdList(requests)) {
    return null;
  }
  return { recording, board: mode, drawers, requests };
}

export function parseBoardOp(v: unknown): BoardOp | null {
  if (!isObj(v)) return null;
  if (v.t === 'clear') return { t: 'clear' };
  if (v.t === 'undo') return isId(v.id) ? { t: 'undo', id: v.id } : null;
  const { id, color, width, points } = v;
  const ok =
    v.t === 'seg' &&
    isId(id) &&
    isColor(color) &&
    isWidth(width) &&
    isPoints(points, LIMITS.segPoints);
  return ok && points.length > 0 ? { t: 'seg', id, color, width, points } : null;
}

export function parseGuestMsg(v: unknown): GuestMsg | null {
  if (!isObj(v)) return null;
  return v.t === 'hello' || v.t === 'draw-request' || v.t === 'draw-cancel' ? { t: v.t } : null;
}

export function parseSnapshot(v: unknown): Stroke[] | null {
  if (!isObj(v) || !Array.isArray(v.strokes) || v.strokes.length > LIMITS.strokes) return null;
  const strokes: Stroke[] = [];
  for (const s of v.strokes as unknown[]) {
    if (!isObj(s)) return null;
    const { id, by, color, width, points } = s;
    if (!isId(id) || !isId(by) || !isColor(color) || !isWidth(width)) return null;
    if (!isPoints(points, LIMITS.strokePoints)) return null;
    strokes.push({ id, by, color, width, points });
  }
  return strokes;
}

/** Applies an already-authorised operation from `by`. Pure. */
export function applyOp(strokes: Stroke[], by: string, op: BoardOp): Stroke[] {
  switch (op.t) {
    case 'clear':
      return [];
    case 'undo':
      return strokes.filter((s) => !(s.id === op.id && s.by === by));
    case 'seg':
      return appendSegment(strokes, by, op);
  }
}

function appendSegment(strokes: Stroke[], by: string, op: Extract<BoardOp, { t: 'seg' }>) {
  const i = strokes.findLastIndex((s) => s.id === op.id);
  const cur = strokes[i];
  if (totalPoints(strokes) + op.points.length > LIMITS.boardPoints) return strokes;
  if (!cur) {
    if (!roomForNewStroke(strokes, by)) return strokes;
    const { id, color, width, points } = op;
    return [...strokes, { id, by, color, width, points }];
  }
  // A stroke belongs to its author; ids from others are ignored.
  if (cur.by !== by || cur.points.length + op.points.length > LIMITS.strokePoints) return strokes;
  const next = strokes.slice();
  next[i] = { ...cur, points: cur.points.concat(op.points) };
  return next;
}

const totalPoints = (strokes: Stroke[]) => strokes.reduce((n, s) => n + s.points.length, 0);

function roomForNewStroke(strokes: Stroke[], by: string): boolean {
  if (strokes.length >= LIMITS.strokes) return false;
  return strokes.filter((s) => s.by === by).length < LIMITS.strokesPerAuthor;
}

/**
 * Combines a snapshot with strokes that arrived live before it: the snapshot
 * wins for ids it contains, newer live strokes are kept.
 */
export function mergeSnapshot(current: Stroke[], snapshot: Stroke[]): Stroke[] {
  const known = new Set(snapshot.map((s) => s.id));
  return [...snapshot, ...current.filter((s) => !known.has(s.id))].slice(0, LIMITS.strokes);
}

/** Whether `identity` may draw under the host's current settings. */
export function canDraw(state: HostState, identity: string, isHost: boolean): boolean {
  if (state.board === 'off') return false;
  return isHost || (state.board === 'collab' && state.drawers.includes(identity));
}
