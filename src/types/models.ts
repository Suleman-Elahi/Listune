import type { DBSchema } from 'idb';

export interface Track {
  id: string;
  title: string;
  artist: string;
  album: string;
  duration: number;
  sourceTag: 's3' | 'local';
  artworkId: string | null;
  s3Key?: string;
  localPath?: string;
}

export interface S3Config {
  endpoint: string;
  bucket: string;
  region: string;
  accessKey: string;
  secretKey: string;
}

export interface LocalSource {
  handle: FileSystemDirectoryHandle;
}

export interface SourceRecord {
  name: string;
  type: 's3' | 'local';
  config: S3Config | { handle: FileSystemDirectoryHandle };
}

export interface ScanProgress {
  scanned: number;
  total: number;
  errors: number;
}

export type WorkerInMessage =
  | { type: 'scan'; source: S3Config | LocalSource }
  | { type: 'cancel' };

export type WorkerOutMessage =
  | { type: 'progress'; scanned: number; total: number; errors: number }
  | { type: 'complete' }
  | { type: 'error'; message: string };

export interface MusicPlayerDB extends DBSchema {
  tracks: {
    key: string;
    value: Track;
    indexes: { 'by-artist': string; 'by-album': string };
  };
  artwork: {
    key: string;
    value: Blob;
  };
  sources: {
    key: string;
    value: SourceRecord;
  };
}
