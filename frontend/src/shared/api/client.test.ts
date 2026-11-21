import { ApiError, errorMessage, request } from './client';

function mockFetch(status: number, body: unknown, headers: Record<string, string> = {}) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(status === 204 ? null : JSON.stringify(body), { status, headers }),
    ),
  );
}

afterEach(() => vi.unstubAllGlobals());

describe('request', () => {
  it('returns parsed JSON', async () => {
    mockFetch(200, { ok: true });
    await expect(request('GET', '/x')).resolves.toEqual({ ok: true });
  });

  it('maps problem details to ApiError with Retry-After', async () => {
    mockFetch(429, { code: 'rate_limited', title: 'Slow down' }, { 'Retry-After': '7' });
    const err = await request('POST', '/x', {}).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 429, code: 'rate_limited', retryAfterSeconds: 7 });
    expect(errorMessage(err)).toBe('Slow down');
  });

  it('handles 204 without a body', async () => {
    mockFetch(204, null);
    await expect(request('POST', '/x')).resolves.toBeUndefined();
  });

  it('sends JSON with same-origin credentials', async () => {
    mockFetch(200, {});
    await request('POST', '/meetings', { a: 1 });
    const [url, init] = vi.mocked(fetch).mock.calls[0]!;
    expect(url).toBe('/api/v1/meetings');
    expect(init).toMatchObject({ method: 'POST', credentials: 'same-origin', body: '{"a":1}' });
  });
});
