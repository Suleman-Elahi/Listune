<template>
  <q-dialog v-model="open" persistent>
    <q-card style="min-width: 340px; background: #1a1a2e; color: white">
      <q-card-section class="row items-center q-pb-none">
        <div class="text-h6">Add Music Source</div>
        <q-space />
        <q-btn icon="close" flat round dense @click="open = false" />
      </q-card-section>

      <q-card-section v-if="savedSources.length" class="q-py-none">
        <div class="text-caption text-grey-5 q-mb-xs">Saved sources</div>
        <q-list dense dark separator>
          <q-item v-for="source in savedSources" :key="source.name" class="q-px-none">
            <q-item-section avatar>
              <q-icon :name="source.type === 's3' ? 'cloud' : 'folder'" size="18px" />
            </q-item-section>
            <q-item-section>
              <q-item-label style="word-break: break-all">{{ source.name }}</q-item-label>
              <q-item-label caption>
                {{ source.type === 's3' ? 'S3 bucket' : 'Local folder' }}
              </q-item-label>
            </q-item-section>
            <q-item-section side>
              <div class="row items-center">
                <q-btn
                  v-if="source.type === 's3'"
                  flat
                  round
                  dense
                  size="sm"
                  icon="edit"
                  @click="useSource(source)"
                >
                  <q-tooltip>Load into form</q-tooltip>
                </q-btn>
                <q-btn flat round dense size="sm" icon="refresh" @click="rescanSource(source)">
                  <q-tooltip>Rescan</q-tooltip>
                </q-btn>
                <q-btn
                  flat
                  round
                  dense
                  size="sm"
                  icon="delete"
                  color="negative"
                  @click="removeSource(source)"
                >
                  <q-tooltip>Remove source and its tracks</q-tooltip>
                </q-btn>
              </div>
            </q-item-section>
          </q-item>
        </q-list>
      </q-card-section>

      <q-card-section>
        <q-tabs v-model="sourceType" dense align="justify" active-color="white" indicator-color="white">
          <q-tab name="server" label="Server Folder" icon="dns" />
          <q-tab name="local" label="Local Folder" icon="folder" />
          <q-tab name="s3" label="S3 / Cloud" icon="cloud" />
        </q-tabs>

        <q-tab-panels v-model="sourceType" animated style="background: transparent">
          <!-- Server folder -->
          <q-tab-panel name="server" class="q-pa-none q-pt-md">
            <p class="text-grey-4 text-sm">
              Enter the absolute path to a folder on the server. The backend will scan it for audio files.
            </p>
            <q-input
              v-model="serverFolder"
              dark
              outlined
              dense
              label="Server folder path"
              placeholder="/home/user/Music"
              class="q-mb-sm"
            />
            <div v-if="serverScanError" class="text-negative q-mt-xs" style="font-size: 12px;">
              {{ serverScanError }}
            </div>
            <div v-if="serverScanning" class="q-mt-sm text-grey-4" style="font-size: 13px;">
              <q-spinner size="14px" class="q-mr-xs" /> Scanning...
            </div>
          </q-tab-panel>

          <!-- Local folder -->
          <q-tab-panel name="local" class="q-pa-none q-pt-md">
            <p class="text-grey-4 text-sm">
              Pick a folder from your device. The app will scan all audio files inside it.
            </p>
            <q-btn
              outline
              color="white"
              icon="folder_open"
              label="Choose Folder"
              class="full-width"
              @click="pickFolder"
            />
            <div v-if="localHandle" class="q-mt-sm text-positive">
              <q-icon name="check_circle" /> {{ localHandle.name }}
            </div>
          </q-tab-panel>

          <!-- S3 -->
          <q-tab-panel name="s3" class="q-pa-none q-pt-md">
            <q-input
              v-model="s3Form.endpoint"
              dark
              outlined
              dense
              label="Endpoint URL"
              placeholder="https://s3.us-east-1.amazonaws.com"
              class="q-mb-sm"
            />
            <q-input v-model="s3Form.bucket" dark outlined dense label="Bucket" class="q-mb-sm" />
            <q-input v-model="s3Form.region" dark outlined dense label="Region" placeholder="us-east-1" class="q-mb-sm" />
            <q-input v-model="s3Form.accessKey" dark outlined dense label="Access Key" class="q-mb-sm" />
            <q-input
              v-model="s3Form.secretKey"
              dark
              outlined
              dense
              label="Secret Key"
              :type="showSecret ? 'text' : 'password'"
              class="q-mb-sm"
            >
              <template #append>
                <q-icon
                  :name="showSecret ? 'visibility_off' : 'visibility'"
                  class="cursor-pointer"
                  @click="showSecret = !showSecret"
                />
              </template>
            </q-input>
            <q-input
              v-model="s3Form.sessionToken"
              dark
              outlined
              dense
              label="Session Token (optional)"
              class="q-mb-sm"
            />
          </q-tab-panel>
        </q-tab-panels>
      </q-card-section>

      <q-card-actions align="right">
        <q-btn flat label="Cancel" color="grey" @click="open = false" />
        <q-btn
          unelevated
          label="Scan"
          color="primary"
          :disable="!canScan"
          :loading="serverScanning"
          @click="startScan"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useLibraryStore } from 'src/stores/libraryStore';
import { db } from 'src/services/db';
import { backend } from 'src/services/backend';
import { s3SourceId } from 'src/services/sigv4';
import type { S3Config, LocalSource, SourceRecord, Track } from 'src/types/models';

const open = defineModel<boolean>({ default: false });

const $q = useQuasar();
const libraryStore = useLibraryStore();
const savedSources = ref<SourceRecord[]>([]);
const sourceType = ref<'server' | 'local' | 's3'>('server');
const showSecret = ref(false);

// Server folder
const serverFolder = ref('');
const serverScanning = ref(false);
const serverScanError = ref('');

// Local
const localHandle = ref<FileSystemDirectoryHandle | null>(null);

async function pickFolder() {
  try {
    // @ts-expect-error showDirectoryPicker is not in all TS lib versions
    const handle = await window.showDirectoryPicker({ mode: 'read' });
    localHandle.value = handle;
  } catch {
    // user cancelled
  }
}

// S3
const s3Form = ref<S3Config>({
  endpoint: '',
  bucket: '',
  region: '',
  accessKey: '',
  secretKey: '',
  sessionToken: '',
});

async function loadSources(): Promise<void> {
  savedSources.value = await db.getSources();
}

watch(
  open,
  (isOpen) => {
    if (isOpen) loadSources().catch(() => undefined);
  },
  { immediate: true },
);

function useSource(source: SourceRecord): void {
  if (source.type !== 's3' || 'handle' in source.config) return;
  const cfg = source.config;
  s3Form.value = { ...cfg, sessionToken: cfg.sessionToken ?? '' };
  sourceType.value = 's3';
}

function rescanSource(source: SourceRecord): void {
  if (source.type === 's3' && !('handle' in source.config)) {
    libraryStore.startScan(source.config, source.name);
  } else if ('handle' in source.config) {
    libraryStore.startScan({ handle: source.config.handle }, source.name);
  }
  open.value = false;
}

function removeSource(source: SourceRecord): void {
  $q.dialog({
    title: 'Remove source',
    message: `Remove "${source.name}" and all tracks scanned from it?`,
    cancel: true,
    persistent: true,
    dark: true,
  }).onOk(() => {
    void (async () => {
      await db.deleteSource(source.name);
      await loadSources();
      await libraryStore.search(libraryStore.searchQuery);
    })();
  });
}

const canScan = computed(() => {
  if (sourceType.value === 'server') return serverFolder.value.trim().length > 0;
  if (sourceType.value === 'local') return !!localHandle.value;
  const f = s3Form.value;
  return !!(f.endpoint && f.bucket && f.region && f.accessKey && f.secretKey);
});

async function startScan() {
  if (sourceType.value === 'server') {
    await scanServerFolder();
  } else if (sourceType.value === 'local' && localHandle.value) {
    const source: LocalSource = { handle: localHandle.value };
    const sourceName = localHandle.value.name;
    await db.putSource(sourceName, {
      name: sourceName,
      type: 'local',
      config: { handle: localHandle.value },
    });
    await loadSources();
    libraryStore.startScan(source, sourceName);
    open.value = false;
  } else if (sourceType.value === 's3') {
    const cfg = { ...s3Form.value };
    const sourceName = s3SourceId(cfg);
    // Save S3 source config to IndexedDB so playback can use it later
    await db.putSource(sourceName, {
      name: sourceName,
      type: 's3' as const,
      config: cfg,
    });
    await loadSources();
    libraryStore.startScan(cfg, sourceName);
    open.value = false;
  }
}

async function scanServerFolder() {
  serverScanError.value = '';
  serverScanning.value = true;

  try {
    const result = await backend.scanServerFolder(serverFolder.value.trim());

    if (result.error) {
      serverScanError.value = result.error;
      serverScanning.value = false;
      return;
    }

    // Convert server tracks to our Track format and store in IndexedDB
    const tracks: Track[] = result.tracks.map((t) => ({
      id: t.id,
      title: t.title,
      artist: t.artist || '',
      album: t.album || '',
      duration: t.duration || 0,
      sourceTag: 'server' as const,
      artworkId: t.hasArtwork ? t.id : null,
      serverPath: t.serverPath,
    }));

    // Batch insert into IndexedDB
    await db.putTracks(tracks);

    // Refresh library
    await libraryStore.search('');

    serverScanning.value = false;
    open.value = false;
  } catch {
    serverScanError.value = 'Failed to connect to backend';
    serverScanning.value = false;
  }
}
</script>
