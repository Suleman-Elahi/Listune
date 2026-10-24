import { db } from 'src/services/db';
import { buildPresignedUrl } from 'src/services/sigv4';
import type { S3Config, Track } from 'src/types/models';

/**
 * Resolve the saved S3 credentials for a track. Tracks store their source name
 * so multiple buckets/endpoints can coexist; fall back to the first S3 source
 * for libraries scanned before source names were recorded.
 */
export async function resolveS3Config(sourceName?: string): Promise<S3Config | null> {
  const sources = await db.getSources();
  const s3Sources = sources.filter((s) => s.type === 's3');
  const source =
    (sourceName ? s3Sources.find((s) => s.name === sourceName) : undefined) ??
    (s3Sources.length === 1 ? s3Sources[0] : undefined);
  return source ? (source.config as S3Config) : null;
}

/** Build a fresh presigned URL for an S3 track. */
export async function getPresignedTrackUrl(track: Track): Promise<string | null> {
  if (track.sourceTag !== 's3' || !track.s3Key) return null;
  const cfg = await resolveS3Config(track.sourceName);
  if (!cfg) return null;
  return buildPresignedUrl(cfg, track.s3Key);
}
