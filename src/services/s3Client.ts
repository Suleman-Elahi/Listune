import type { S3Config } from 'src/types/models';
import { buildPresignedUrl, proxyUrlFor } from 'src/services/sigv4';

// Stateless S3 helpers. Credentials are passed explicitly per source so that
// multiple buckets/endpoints can coexist without a stale module-level config.

async function getPresignedUrl(cfg: S3Config, key: string, expiresIn = 3600): Promise<string> {
  return buildPresignedUrl(cfg, key, { expiresIn });
}

async function fetchPartial(
  cfg: S3Config,
  key: string,
  bytes = 131072,
): Promise<ArrayBuffer> {
  const presigned = await getPresignedUrl(cfg, key);
  const response = await fetch(proxyUrlFor(presigned), {
    headers: { Range: `bytes=0-${bytes - 1}` },
  });
  if (!response.ok && response.status !== 206) {
    throw new Error(`S3 partial fetch failed: ${response.status} ${response.statusText}`);
  }
  return response.arrayBuffer();
}

export const s3Client = { getPresignedUrl, fetchPartial };
