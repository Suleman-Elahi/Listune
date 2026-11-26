import { afterEach, describe, expect, it, vi } from 'vitest';
import { youtube } from 'src/services/youtube';

function stubFetch(body: unknown): ReturnType<typeof vi.fn> {
  const fn = vi.fn(() => Promise.resolve(new Response(JSON.stringify(body), { status: 200 })));
  vi.stubGlobal('fetch', fn);
  return fn;
}

describe('youtube service', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('passes the Google session handle and query to search', async () => {
    const fetchMock = stubFetch({ searchId: 's1', query: 'test', results: [] });

    await expect(youtube.search('test', 'handle.abc')).resolves.toMatchObject({ searchId: 's1' });

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toContain('/api/youtube/search?token=handle.abc');
    expect(JSON.parse(init.body as string)).toEqual({ query: 'test' });
  });

  it('sends cached video ids for download', async () => {
    const fetchMock = stubFetch({ jobId: 'j1', files: 2 });

    await expect(
      youtube.download('s1', ['abcdefghijk', 'abcdefghijk'], 'handle.abc', '320'),
    ).resolves.toMatchObject({ jobId: 'j1' });

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toContain('/api/youtube/download?token=handle.abc');
    expect(JSON.parse(init.body as string)).toEqual({
      searchId: 's1',
      videos: [{ videoId: 'abcdefghijk' }, { videoId: 'abcdefghijk' }],
      quality: '320',
    });
  });

  it('treats a null job listing as empty', async () => {
    stubFetch({ jobs: null });
    await expect(youtube.jobs()).resolves.toEqual([]);
  });
});
