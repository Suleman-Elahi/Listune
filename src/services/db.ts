import { openDB, type IDBPDatabase } from 'idb';
import type { Track, SourceRecord, MusicPlayerDB } from 'src/types/models';

export interface TrackFilter {
  search?: string;
  limit?: number;
  offset?: number;
}

let dbPromise: Promise<IDBPDatabase<MusicPlayerDB>> | null = null;

function getDB(): Promise<IDBPDatabase<MusicPlayerDB>> {
  if (!dbPromise) {
    dbPromise = openDB<MusicPlayerDB>('music-player-db', 1, {
      upgrade(db) {
        const trackStore = db.createObjectStore('tracks', { keyPath: 'id' });
        trackStore.createIndex('by-artist', 'artist');
        trackStore.createIndex('by-album', 'album');

        db.createObjectStore('artwork');
        db.createObjectStore('sources', { keyPath: 'name' });
      },
    });
  }
  return dbPromise;
}

export const db = {
  async putTrack(track: Track): Promise<void> {
    const database = await getDB();
    await database.put('tracks', track);
  },

  async putTracks(tracks: Track[]): Promise<void> {
    const database = await getDB();
    const tx = database.transaction('tracks', 'readwrite');
    await Promise.all([...tracks.map((t) => tx.store.put(t)), tx.done]);
  },

  async getTrack(id: string): Promise<Track | undefined> {
    const database = await getDB();
    return database.get('tracks', id);
  },

  async queryTracks(filter: TrackFilter): Promise<Track[]> {
    const database = await getDB();
    const all = await database.getAll('tracks');

    let results = all;
    if (filter.search) {
      const q = filter.search.toLowerCase();
      results = all.filter(
        (t) =>
          t.title.toLowerCase().includes(q) ||
          t.artist.toLowerCase().includes(q) ||
          t.album.toLowerCase().includes(q),
      );
    }

    const offset = filter.offset ?? 0;
    const sliced = results.slice(offset);
    return filter.limit !== undefined ? sliced.slice(0, filter.limit) : sliced;
  },

  async putArtwork(trackId: string, blob: Blob): Promise<void> {
    const database = await getDB();
    await database.put('artwork', blob, trackId);
  },

  async getArtwork(trackId: string): Promise<Blob | undefined> {
    const database = await getDB();
    return database.get('artwork', trackId);
  },

  async putSource(name: string, record: SourceRecord): Promise<void> {
    const database = await getDB();
    await database.put('sources', { ...record, name });
  },

  async getSources(): Promise<SourceRecord[]> {
    const database = await getDB();
    return database.getAll('sources');
  },
};
