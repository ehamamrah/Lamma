import {
  applyOp,
  canDraw,
  mergeSnapshot,
  initialHostState,
  parseBoardOp,
  parseHostState,
  parseSnapshot,
} from './protocol';

const seg = (id: string, points: number[]) =>
  ({ t: 'seg', id, color: '#111827', width: 5, points }) as const;

describe('parseBoardOp', () => {
  test('accepts a valid segment', () => {
    expect(parseBoardOp(seg('s1', [0, 0.5, 1, 1]))).toEqual(seg('s1', [0, 0.5, 1, 1]));
  });
  test.each([
    ['out-of-range point', seg('s1', [0, 2])],
    ['odd point count', seg('s1', [0.1])],
    ['empty points', seg('s1', [])],
    ['unknown colour', { ...seg('s1', [0, 0]), color: 'red; x' }],
    ['unknown width', { ...seg('s1', [0, 0]), width: 99 }],
    ['unknown type', { t: 'nuke' }],
  ])('rejects %s', (_, msg) => expect(parseBoardOp(msg)).toBeNull());
});

test('parseHostState validates every field', () => {
  const ok = { t: 'state', recording: true, board: 'collab', drawers: ['a'], requests: [] };
  expect(parseHostState(ok)).toEqual({
    recording: true,
    board: 'collab',
    drawers: ['a'],
    requests: [],
  });
  expect(parseHostState({ ...ok, board: 'all' })).toBeNull();
  expect(parseHostState({ ...ok, drawers: [1] })).toBeNull();
});

test('parseSnapshot rejects any malformed stroke', () => {
  const s = { id: 's', by: 'a', color: '#111827', width: 2, points: [0, 0] };
  expect(parseSnapshot({ strokes: [s] })).toEqual([s]);
  expect(parseSnapshot({ strokes: [s, { ...s, points: [5, 5] }] })).toBeNull();
});

describe('applyOp', () => {
  test('appends segments to the author’s own stroke only', () => {
    let strokes = applyOp([], 'alice', seg('s1', [0, 0]));
    strokes = applyOp(strokes, 'alice', seg('s1', [1, 1]));
    strokes = applyOp(strokes, 'mallory', seg('s1', [0.5, 0.5]));
    expect(strokes).toHaveLength(1);
    expect(strokes[0]?.points).toEqual([0, 0, 1, 1]);
  });
  test('undo removes only the author’s stroke; clear removes all', () => {
    const strokes = applyOp(applyOp([], 'a', seg('s1', [0, 0])), 'b', seg('s2', [0, 0]));
    expect(applyOp(strokes, 'b', { t: 'undo', id: 's1' })).toHaveLength(2);
    expect(applyOp(strokes, 'a', { t: 'undo', id: 's1' })).toHaveLength(1);
    expect(applyOp(strokes, 'a', { t: 'clear' })).toEqual([]);
  });
});

test('canDraw follows the host’s mode and approvals', () => {
  const collab = { ...initialHostState, board: 'collab' as const, drawers: ['g1'] };
  expect(canDraw(initialHostState, 'host', true)).toBe(false);
  expect(canDraw({ ...collab, board: 'solo' }, 'g1', false)).toBe(false);
  expect(canDraw({ ...collab, board: 'solo' }, 'host', true)).toBe(true);
  expect(canDraw(collab, 'g1', false)).toBe(true);
  expect(canDraw(collab, 'g2', false)).toBe(false);
});

test('mergeSnapshot keeps live strokes the snapshot does not have', () => {
  const a = { id: 'a', by: 'h', color: '#111827', width: 2, points: [0, 0] };
  const b = { ...a, id: 'b' };
  const stale = { ...a, points: [1, 1] };
  expect(mergeSnapshot([stale, b], [a])).toEqual([a, b]);
});

test('one author cannot exceed their stroke allowance', () => {
  let strokes = applyOp([], 'a', seg('s0', [0, 0]));
  for (let i = 1; i < 400; i++) strokes = applyOp(strokes, 'a', seg(`s${i}`, [0, 0]));
  expect(strokes).toHaveLength(300);
  expect(applyOp(strokes, 'b', seg('b1', [0, 0]))).toHaveLength(301);
});
