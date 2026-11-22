import { boardHost, useCollabStore } from './collab';

beforeEach(() => useCollabStore.getState().reset());
const host = () => useCollabStore.getState().host;

test('draw requests are only accepted in collaborative mode', () => {
  boardHost.open('solo');
  boardHost.request('g1');
  expect(host().requests).toEqual([]);
  boardHost.setMode('collab');
  boardHost.request('g1');
  boardHost.request('g1');
  expect(host().requests).toEqual(['g1']);
});

test('approve moves a request to drawers; revoke and close clean up', () => {
  boardHost.open('collab');
  boardHost.request('g1');
  boardHost.approve('g1');
  expect(host()).toMatchObject({ drawers: ['g1'], requests: [] });
  boardHost.revoke('g1');
  expect(host().drawers).toEqual([]);
  boardHost.request('g2');
  boardHost.close();
  expect(host()).toMatchObject({ board: 'off', requests: [] });
});
