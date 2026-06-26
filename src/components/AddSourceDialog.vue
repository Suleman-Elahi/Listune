<template>
  <q-dialog v-model="open" persistent>
    <q-card style="min-width: 340px; background: #1a1a2e; color: white">
      <q-card-section class="row items-center q-pb-none">
        <div class="text-h6">Add Music Source</div>
        <q-space />
        <q-btn icon="close" flat round dense @click="open = false" />
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
              placeholder="https://s3.amazonaws.com"
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
import { ref, computed } from 'vue';
import { useLibraryStore } from 'src/stores/libraryStore';
import { db } from 'src/services/db';
import { backend } from 'src/services/backend';
import type { S3Config, LocalSource, Track } from 'src/types/models';

const open = defineModel<boolean>({ default: false });

const libraryStore = useLibraryStore();
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
});

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
    void db.putSource(localHandle.value.name, {
      name: localHandle.value.name,
      type: 'local',
      config: { handle: localHandle.value },
    });
    libraryStore.startScan(source);
    open.value = false;
  } else if (sourceType.value === 's3') {
    libraryStore.startScan({ ...s3Form.value });
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
  } catch (err) {
    serverScanError.value = 'Failed to connect to backend';
    serverScanning.value = false;
  }
}
</script>
