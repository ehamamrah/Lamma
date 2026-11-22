import { fitBoard, normalise } from './draw';

test('fitBoard keeps 16:9 inside the container', () => {
  expect(fitBoard(1600, 2000)).toEqual({ w: 1600, h: 900 });
  expect(fitBoard(2000, 900)).toEqual({ w: 1600, h: 900 });
});

test('normalise clamps and rounds', () => {
  const rect = { left: 10, top: 10, width: 100, height: 50 } as DOMRect;
  expect(normalise(60, 35, rect)).toEqual([0.5, 0.5]);
  expect(normalise(-50, 500, rect)).toEqual([0, 1]);
  expect(normalise(43.33333, 10, rect)).toEqual([0.3333, 0]);
});
