import { webcrypto } from 'node:crypto';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { S3Config } from 'src/types/models';
import {
  buildPresignedUrl,
  canonicalS3Path,
  encodeS3Key,
  normalizeEndpoint,
  s3SourceId,
  stableTrackId,
} from 'src/services/sigv4';

if (!globalThis.crypto?.subtle) {
  Object.defineProperty(globalThis, 'crypto', { value: webcrypto, configurable: true });
}

const cfg: S3Config = {
  endpoint: 'https://s3.us-east-1.amazonaws.com/',
  bucket: 'music',
  region: 'us-east-1',
  accessKey: 'AKIAEXAMPLE',
  secretKey: 'secret',
};

describe('sigv4 helpers', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-10-24T12:00:00.000Z'));
  });

  it('normalizes trailing slashes on endpoints', () => {
    expect(normalizeEndpoint('https://host/').pathname).toBe('/');
    expect(normalizeEndpoint('https://host/s3/').pathname).toBe('/s3');
  });

  it('RFC3986-encodes object keys without escaping path separators', () => {
    expect(encodeS3Key('a b/c+d#e')).toBe('a%20b/c%2Bd%23e');
  });

  it('includes the endpoint path prefix and encoded key in the canonical path', () => {
    const prefixed: S3Config = { ...cfg, endpoint: 'https://host/s3/' };
    expect(canonicalS3Path(prefixed, 'folder/a b.mp3')).toBe('/s3/music/folder/a%20b.mp3');
  });

  it('signs bucket-level list requests and encoded keys', async () => {
    const listUrl = await buildPresignedUrl(cfg, '', {
      expiresIn: 60,
      query: { 'list-type': '2', 'encoding-type': 'url' },
    });
    expect(listUrl).toContain('https://s3.us-east-1.amazonaws.com/music?');
    expect(listUrl).toContain('X-Amz-Signature=');

    const objectUrl = await buildPresignedUrl(cfg, 'folder/a b.mp3');
    expect(objectUrl).toContain('/music/folder/a%20b.mp3?');
  });

  it('includes session tokens in the signed query', async () => {
    const url = await buildPresignedUrl({ ...cfg, sessionToken: 'token/with+chars' }, 'song.mp3');
    expect(url).toContain('X-Amz-Security-Token=token%2Fwith%2Bchars');
  });

  it('derives stable source and track IDs', async () => {
    expect(s3SourceId(cfg)).toBe('s3.us-east-1.amazonaws.com/music');
    const first = await stableTrackId('source', 's3', 'song.mp3');
    const second = await stableTrackId('source', 's3', 'song.mp3');
    expect(first).toBe(second);
    expect(first).toMatch(/^[0-9a-f]{32}$/);
  });
});
