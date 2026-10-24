import type { S3Config } from 'src/types/models';

// Shared AWS SigV4 helpers used by both the main thread and the scanner worker.
// Keeping this in one place prevents the two copies from drifting apart.

const encoder = new TextEncoder();

function toHex(buf: ArrayBuffer): string {
  return Array.from(new Uint8Array(buf))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
}

async function hmacSha256(key: ArrayBuffer | Uint8Array, data: string): Promise<ArrayBuffer> {
  const keyBytes = key instanceof Uint8Array ? key : new Uint8Array(key);
  const cryptoKey = await crypto.subtle.importKey(
    'raw',
    keyBytes.buffer.slice(keyBytes.byteOffset, keyBytes.byteOffset + keyBytes.byteLength) as ArrayBuffer,
    { name: 'HMAC', hash: 'SHA-256' },
    false,
    ['sign'],
  );
  return crypto.subtle.sign('HMAC', cryptoKey, encoder.encode(data));
}

export async function sha256Hex(data: string | Uint8Array): Promise<string> {
  const bytes = typeof data === 'string' ? encoder.encode(data) : data;
  const buf = await crypto.subtle.digest('SHA-256', bytes as BufferSource);
  return toHex(buf);
}

export function formatDate(d: Date): string {
  return d.toISOString().slice(0, 10).replace(/-/g, '');
}

export function formatDateTime(d: Date): string {
  return d.toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '');
}

/** RFC 3986 encoding (encodeURIComponent leaves a few characters unescaped that AWS requires escaped). */
export function encodeRfc3986(value: string): string {
  return encodeURIComponent(value).replace(/[!'()*]/g, (c) => {
    return `%${c.charCodeAt(0).toString(16).toUpperCase()}`;
  });
}

/** Encode an object key without encoding the path separators. */
export function encodeS3Key(key: string): string {
  return key.split('/').map(encodeRfc3986).join('/');
}

/** Normalize an endpoint: require http(s), drop query/fragment, trim trailing slashes. */
export function normalizeEndpoint(endpoint: string): URL {
  let url: URL;
  try {
    url = new URL(endpoint);
  } catch {
    throw new Error(`S3 endpoint is not a valid URL: ${endpoint}`);
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    throw new Error('S3 endpoint must use http or https');
  }
  url.hash = '';
  url.search = '';
  url.pathname = url.pathname.replace(/\/+$/, '') || '/';
  return url;
}

/** The escaped path prefix of the endpoint (supports endpoints mounted under a path). */
function endpointPathPrefix(endpoint: URL): string {
  return endpoint.pathname === '/' ? '' : endpoint.pathname.replace(/\/+$/, '');
}

/** Canonical URI for a bucket and optional object key. */
export function canonicalS3Path(cfg: S3Config, key?: string): string {
  const endpoint = normalizeEndpoint(cfg.endpoint);
  const prefix = endpointPathPrefix(endpoint);
  const parts = [encodeRfc3986(cfg.bucket)];
  if (key !== undefined && key !== '') {
    parts.push(encodeS3Key(key));
  }
  return `${prefix}/${parts.join('/')}`;
}

export interface PresignOptions {
  expiresIn?: number;
  method?: string;
  query?: Record<string, string>;
}

/**
 * Build a SigV4 query-authenticated (presigned) URL.
 * `key` may be empty to sign a bucket-level request (e.g. ListObjectsV2).
 */
export async function buildPresignedUrl(
  cfg: S3Config,
  key: string,
  options: PresignOptions = {},
): Promise<string> {
  const required: (keyof S3Config)[] = ['endpoint', 'bucket', 'region', 'accessKey', 'secretKey'];
  for (const field of required) {
    if (!cfg[field]) {
      throw new Error(`S3Client: missing required field "${field}"`);
    }
  }

  const method = options.method ?? 'GET';
  const expiresIn = options.expiresIn ?? 3600;
  const endpoint = normalizeEndpoint(cfg.endpoint);
  const host = endpoint.host;
  const canonicalUri = canonicalS3Path(cfg, key);
  const now = new Date();
  const dateStamp = formatDate(now);
  const amzDate = formatDateTime(now);
  const credentialScope = `${dateStamp}/${cfg.region}/s3/aws4_request`;
  const credential = `${cfg.accessKey}/${credentialScope}`;

  const query: Record<string, string> = {
    'X-Amz-Algorithm': 'AWS4-HMAC-SHA256',
    'X-Amz-Credential': credential,
    'X-Amz-Date': amzDate,
    'X-Amz-Expires': String(expiresIn),
    'X-Amz-SignedHeaders': 'host',
    ...(options.query ?? {}),
  };
  if (cfg.sessionToken) {
    query['X-Amz-Security-Token'] = cfg.sessionToken;
  }

  const canonicalQueryString = Object.keys(query)
    .sort()
    .map((k) => `${encodeRfc3986(k)}=${encodeRfc3986(query[k]!)}`)
    .join('&');

  const canonicalHeaders = `host:${host}\n`;
  const signedHeaders = 'host';
  const payloadHash = 'UNSIGNED-PAYLOAD';

  const canonicalRequest = [
    method,
    canonicalUri,
    canonicalQueryString,
    canonicalHeaders,
    signedHeaders,
    payloadHash,
  ].join('\n');

  const canonicalRequestHash = await sha256Hex(canonicalRequest);
  const stringToSign = [
    'AWS4-HMAC-SHA256',
    amzDate,
    credentialScope,
    canonicalRequestHash,
  ].join('\n');

  const kSecret = encoder.encode(`AWS4${cfg.secretKey}`);
  const kDate = await hmacSha256(kSecret, dateStamp);
  const kRegion = await hmacSha256(kDate, cfg.region);
  const kService = await hmacSha256(kRegion, 's3');
  const kSigning = await hmacSha256(kService, 'aws4_request');
  const signature = toHex(await hmacSha256(kSigning, stringToSign));

  return `${endpoint.origin}${canonicalUri}?${canonicalQueryString}&X-Amz-Signature=${signature}`;
}

function backendBaseUrl(): string {
  return import.meta.env.VITE_BACKEND_URL || 'http://localhost:8090';
}

/** Route a presigned URL through the backend proxy to sidestep CORS. */
export function proxyUrlFor(presignedUrl: string): string {
  return `${backendBaseUrl()}/api/s3proxy?url=${encodeURIComponent(presignedUrl)}`;
}

/** Stable identifier for a source, used to resolve credentials later. */
export function s3SourceId(cfg: S3Config): string {
  try {
    const endpoint = normalizeEndpoint(cfg.endpoint);
    const prefix = endpoint.pathname.replace(/\/+$/, '');
    return `${endpoint.host}${prefix}/${cfg.bucket}`;
  } catch {
    return cfg.bucket;
  }
}

/** Deterministic track ID so re-scanning upserts instead of duplicating. */
export async function stableTrackId(...parts: string[]): Promise<string> {
  return (await sha256Hex(parts.join('\u0000'))).slice(0, 32);
}
