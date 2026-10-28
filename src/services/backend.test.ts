import { afterEach, describe, expect, it, vi } from 'vitest';
import { backend, BackendHttpError, isAuthError } from 'src/services/backend';

function stubFetch(status: number, body: unknown): void {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(new Response(JSON.stringify(body), { status }))),
  );
}

describe('backend drive calls', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('reports 401 as an auth error so the UI can prompt a re-login', async () => {
    stubFetch(401, { error: 'Google session expired. Sign in again.' });

    let caught: unknown;
    try {
      await backend.gdriveFolders('root', 'handle');
    } catch (err) {
      caught = err;
    }

    expect(caught).toBeInstanceOf(BackendHttpError);
    expect(isAuthError(caught)).toBe(true);
    expect((caught as BackendHttpError).status).toBe(401);
    expect((caught as Error).message).toContain('Google session expired');
  });

  it('treats a JSON null listing as an empty array', async () => {
    stubFetch(200, null);
    await expect(backend.gdriveFolders('root', 'handle')).resolves.toEqual([]);
    await expect(backend.gdriveFiles('root', 'handle')).resolves.toEqual([]);
  });

  it('returns the listing as-is when it is a real array', async () => {
    const folders = [{ id: '1', name: 'Music', mimeType: 'application/vnd.google-apps.folder' }];
    stubFetch(200, folders);
    await expect(backend.gdriveFolders('root', 'handle')).resolves.toEqual(folders);
  });

  it('does not treat plain errors as auth errors', () => {
    expect(isAuthError(new Error('boom'))).toBe(false);
    expect(isAuthError(new BackendHttpError(502, 'bad gateway'))).toBe(false);
    expect(isAuthError(undefined)).toBe(false);
  });
});
