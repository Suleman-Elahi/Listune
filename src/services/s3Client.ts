import type { S3Config } from 'src/types/models';

let config: S3Config | null = null;

function configure(cfg: S3Config): void {
  const required: (keyof S3Config)[] = ['endpoint', 'bucket', 'region', 'accessKey', 'secretKey'];
  for (const field of required) {
    if (!cfg[field]) {
      throw new Error(`S3Client: missing required field "${field}"`);
    }
  }
  config = cfg;
}

function getConfig(): S3Config {
  if (!config) throw new Error('S3Client: not configured — call configure() first');
  return config;
}

// SubtleCrypto helpers

async function hmacSha256(key: ArrayBuffer, data: string): Promise<ArrayBuffer> {
  const cryptoKey = await crypto.subtle.importKey(
    'raw',
    key,
    { name: 'HMAC', hash: 'SHA-256' },
    false,
    ['sign'],
  );
  return crypto.subtle.sign('HMAC', cryptoKey, new TextEncoder().encode(data));
}

async function sha256Hex(data: string): Promise<string> {
  const buf = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(data));
  return Array.from(new Uint8Array(buf))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
}

function toHex(buf: ArrayBuffer): string {
  return Array.from(new Uint8Array(buf))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
}

function formatDate(d: Date): string {
  // YYYYMMDD
  return d.toISOString().slice(0, 10).replace(/-/g, '');
}

function formatDateTime(d: Date): string {
  // YYYYMMDDTHHmmssZ
  return d.toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '');
}

async function getPresignedUrl(key: string, expiresIn = 3600): Promise<string> {
  const cfg = getConfig();
  const now = new Date();
  const dateStamp = formatDate(now);
  const amzDate = formatDateTime(now);

  const credentialScope = `${dateStamp}/${cfg.region}/s3/aws4_request`;
  const credential = `${cfg.accessKey}/${credentialScope}`;

  // Canonical query string (params must be sorted)
  const queryParams: Record<string, string> = {
    'X-Amz-Algorithm': 'AWS4-HMAC-SHA256',
    'X-Amz-Credential': credential,
    'X-Amz-Date': amzDate,
    'X-Amz-Expires': String(expiresIn),
    'X-Amz-SignedHeaders': 'host',
  };

  const sortedKeys = Object.keys(queryParams).sort();
  const canonicalQueryString = sortedKeys
    .map((k) => `${encodeURIComponent(k)}=${encodeURIComponent(queryParams[k]!)}`)
    .join('&');

  // Derive host from endpoint
  const endpointUrl = new URL(cfg.endpoint);
  const host = endpointUrl.host;

  const canonicalUri = `/${cfg.bucket}/${key}`;
  const canonicalHeaders = `host:${host}\n`;
  const signedHeaders = 'host';
  const payloadHash = 'UNSIGNED-PAYLOAD';

  const canonicalRequest = [
    'GET',
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

  // Derive signing key
  const kSecret = new TextEncoder().encode(`AWS4${cfg.secretKey}`).buffer as ArrayBuffer;
  const kDate = await hmacSha256(kSecret, dateStamp);
  const kRegion = await hmacSha256(kDate, cfg.region);
  const kService = await hmacSha256(kRegion, 's3');
  const kSigning = await hmacSha256(kService, 'aws4_request');

  const signatureBuf = await hmacSha256(kSigning, stringToSign);
  const signature = toHex(signatureBuf);

  const url = `${cfg.endpoint}/${cfg.bucket}/${key}?${canonicalQueryString}&X-Amz-Signature=${signature}`;
  return url;
}

async function fetchPartial(key: string, bytes: number): Promise<ArrayBuffer> {
  const url = await getPresignedUrl(key);
  const response = await fetch(url, {
    headers: { Range: `bytes=0-${bytes - 1}` },
  });
  return response.arrayBuffer();
}

export const s3Client = { configure, getPresignedUrl, fetchPartial };
