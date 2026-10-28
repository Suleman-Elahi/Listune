import 'fake-indexeddb/auto';
import { webcrypto } from 'node:crypto';
import { beforeEach, describe, expect, it } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { db } from 'src/services/db';
import { usePlayerStore } from 'src/stores/playerStore';
import type { SourceRecord, Track } from 'src/types/models';

if (!globalThis.crypto?.subtle) {
  Object.defineProperty(globalThis, 'crypto', { value: webcrypto, configurable: true });
}

function makeTrack(id: string, extra: Partial<Track> = {}): Track {
  return {
    id,
    title: `title-${id}`,
    artist: 'artist',
    album: 'album',
    duration: 100,
    sourceTag: 'server',
    artworkId: null,
    ...extra,
  };
}

describe('playerStore.pruneQueue', () => {
  beforeEach(async () => {
    setActivePinia(createPinia());

    for (const id of ['keep', 'owned']) {
      await db.putTrack(makeTrack(id));
    }
    await db.putTrack(makeTrack('orphan', { sourceName: 'gdrive-gone' }));

    const source: SourceRecord = {
      name: 'bucket1',
      type: 's3',
      config: {
        endpoint: 'https://s3.us-east-1.amazonaws.com',
        bucket: 'music',
        region: 'us-east-1',
        accessKey: 'key',
        secretKey: 'secret',
      },
    };
    await db.putSource(source.name, source);
    await db.putTrack(makeTrack('owned', { sourceTag: 's3', sourceName: 'bucket1', s3Key: 'a.mp3' }));
  });

  it('drops tracks that no longer exist and tracks whose source was removed', async () => {
    const store = usePlayerStore();
    store.setQueue(['keep', 'deleted', 'orphan', 'owned']);

    await store.pruneQueue();

    expect(store.queue).toEqual(['keep', 'owned']);
  });

  it('keeps server tracks that never recorded a source name', async () => {
    const store = usePlayerStore();
    store.setQueue(['keep']);

    await store.pruneQueue();

    expect(store.queue).toEqual(['keep']);
  });

  it('clears a current track that is no longer in the queue', async () => {
    const store = usePlayerStore();
    store.setQueue(['deleted', 'keep']);
    store.currentTrackId = 'deleted';

    await store.pruneQueue();

    expect(store.queue).toEqual(['keep']);
    expect(store.currentTrackId).toBeNull();
  });

  it('leaves an intact queue untouched', async () => {
    const store = usePlayerStore();
    store.setQueue(['keep', 'owned']);
    store.currentTrackId = 'keep';

    await store.pruneQueue();

    expect(store.queue).toEqual(['keep', 'owned']);
    expect(store.currentTrackId).toBe('keep');
  });
});
