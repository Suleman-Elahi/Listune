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
              <q-icon :name="source.type === 's3' ? 'cloud' : source.type === 'gdrive' ? 'cloud_queue' : 'folder'" size="18px" />
            </q-item-section>
            <q-item-section>
              <q-item-label style="word-break: break-all">{{ source.name }}</q-item-label>
              <q-item-label caption>
                {{
                  source.type === 's3'
                    ? 'S3 bucket'
                    : source.type === 'gdrive'
                      ? 'Google Drive folder'
                      : 'Local folder'
                }}
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
          <q-tab name="gdrive" label="Google Drive" icon="cloud_queue" />
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

          <!-- Google Drive -->
          <q-tab-panel name="gdrive" class="q-pa-none q-pt-md">
            <div v-if="!googleToken" class="text-grey-4 text-sm q-mb-sm">
              Sign in with Google to access Drive.
              <q-btn flat dense color="primary" label="Sign in" @click="backend.loginGoogle()" />
            </div>
            <template v-else>
              <p class="text-grey-4 text-sm">
                Pick a folder from your Google Drive. All audio files inside it will be added.
              </p>
              <q-btn
                outline
                color="white"
                icon="folder_open"
                label="Choose Folder"
                class="full-width"
                @click="openFolderPicker"
              />
              <div v-if="gdriveCurrentName" class="q-mt-sm text-positive">
                <q-icon name="check_circle" /> {{ gdriveCurrentName }}
              </div>
              <div v-if="gdriveError" class="text-negative q-mt-xs" style="font-size: 12px;">
                {{ gdriveError }}
              </div>
            </template>
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

  <!-- Google Drive folder picker -->
  <q-dialog v-model="pickerOpen" maximized>
    <q-card class="picker-card">
      <q-card-section class="row items-center q-pb-none">
        <div class="text-h6">Choose a Drive folder</div>
        <q-space />
        <q-btn icon="close" flat round dense @click="pickerOpen = false" />
      </q-card-section>

      <!-- Breadcrumb -->
      <q-card-section class="q-py-sm">
        <div class="row items-center no-wrap q-gutter-xs breadcrumb">
          <q-btn
            flat
            dense
            size="sm"
            icon="home"
            :disable="pickerPath.length === 0"
            @click="goToRoot"
          >
            <q-tooltip>My Drive</q-tooltip>
          </q-btn>
          <template v-for="(crumb, i) in pickerPath" :key="crumb.id">
            <q-icon name="chevron_right" size="16px" color="grey-6" />
            <q-btn
              flat
              dense
              size="sm"
              :label="crumb.name"
              :disable="i === pickerPath.length - 1"
              @click="goToCrumb(i)"
            />
          </template>
        </div>
      </q-card-section>

      <q-separator dark />

      <!-- Folder list -->
      <q-card-section class="q-pt-sm" style="flex: 1; overflow: auto;">
        <div v-if="pickerLoading" class="column items-center q-pa-lg">
          <q-spinner size="28px" color="primary" />
          <div class="text-grey-5 q-mt-sm" style="font-size: 13px;">Loading folders…</div>
        </div>
        <div v-else-if="pickerError" class="column items-center q-pa-lg">
          <q-icon name="error_outline" size="32px" color="negative" />
          <div class="text-negative q-mt-sm" style="font-size: 13px;">{{ pickerError }}</div>
          <div class="row q-mt-sm q-gutter-sm">
            <q-btn
              v-if="pickerErrorStatus === 401"
              unelevated
              color="primary"
              label="Sign in with Google"
              @click="backend.loginGoogle()"
            />
            <q-btn flat dense color="primary" label="Retry" @click="loadPickerFolders" />
          </div>
        </div>
        <div v-else-if="pickerList.length === 0" class="column items-center q-pa-lg">
          <q-icon name="folder_open" size="32px" color="grey-6" />
          <div class="text-grey-5 q-mt-sm" style="font-size: 13px;">No subfolders here</div>
        </div>
        <q-list v-else dense dark separator>
          <q-item
            v-for="f in pickerList"
            :key="f.id"
            clickable
            v-ripple
            @click="enterPickerFolder(f.id, f.name)"
          >
            <q-item-section avatar>
              <q-icon name="folder" size="20px" color="amber-4" />
            </q-item-section>
            <q-item-section>
              <q-item-label>{{ f.name }}</q-item-label>
            </q-item-section>
            <q-item-section side>
              <q-icon name="chevron_right" size="18px" color="grey-6" />
            </q-item-section>
          </q-item>
        </q-list>
      </q-card-section>

      <q-separator dark />

      <q-card-actions align="right">
        <q-btn flat label="Cancel" color="grey" @click="pickerOpen = false" />
        <q-btn
          unelevated
          :label="`Select ${currentPickerFolderName || 'My Drive'}`"
          color="primary"
          @click="confirmPickerFolder"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useLibraryStore } from 'src/stores/libraryStore';
import { usePlayerStore } from 'src/stores/playerStore';
import { db } from 'src/services/db';
import { backend, isAuthError } from 'src/services/backend';
import { s3SourceId } from 'src/services/sigv4';
import type { S3Config, LocalSource, SourceRecord, Track } from 'src/types/models';

const open = defineModel<boolean>({ default: false });

const $q = useQuasar();
const libraryStore = useLibraryStore();
const playerStore = usePlayerStore();
const savedSources = ref<SourceRecord[]>([]);
const sourceType = ref<'server' | 'local' | 's3' | 'gdrive'>('server');
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


// Google Drive
const gdriveFolderId = ref('');
const gdriveCurrentName = ref('');
const gdriveError = ref('');

// Folder picker popup state
const pickerOpen = ref(false);
const pickerLoading = ref(false);
const pickerError = ref('');
/** HTTP status of the last picker failure; 401 means "sign in again". */
const pickerErrorStatus = ref<number | null>(null);
const pickerFolders = ref<{ id: string; name: string; mimeType: string }[]>([]);
/** Navigation stack; empty = My Drive root. */
const pickerPath = ref<{ id: string; name: string }[]>([]);

const currentPickerFolderId = computed(
  () => pickerPath.value[pickerPath.value.length - 1]?.id ?? 'root',
);
const currentPickerFolderName = computed(
  () => pickerPath.value[pickerPath.value.length - 1]?.name ?? '',
);
/** Guaranteed array — a null payload must never reach `.length` in the render. */
const pickerList = computed(() => pickerFolders.value ?? []);

function getGoogleToken(): string | null {
  try {
    const raw = localStorage.getItem('listune_user');
    if (!raw) return null;
    const u = JSON.parse(raw) as { token?: string };
    return u.token ?? null;
  } catch {
    return null;
  }
}
// localStorage is not reactive; refresh when the dialog opens.
const googleToken = ref<string | null>(null);

async function loadPickerFolders(): Promise<void> {
  const token = getGoogleToken();
  if (!token) {
    pickerError.value = 'Not signed in with Google';
    pickerErrorStatus.value = 401;
    return;
  }
  pickerLoading.value = true;
  pickerError.value = '';
  pickerErrorStatus.value = null;
  try {
    const folders = await backend.gdriveFolders(currentPickerFolderId.value, token);
    pickerFolders.value = Array.isArray(folders) ? folders : [];
  } catch (err) {
    pickerFolders.value = [];
    pickerErrorStatus.value = isAuthError(err) ? 401 : null;
    pickerError.value = describePickerError(err);
    if (pickerErrorStatus.value === 401) {
      // Drop the stale handle so the tab offers "Sign in with Google".
      googleToken.value = null;
    }
  } finally {
    pickerLoading.value = false;
  }
}

function describePickerError(err: unknown): string {
  if (isAuthError(err)) return 'Your Google session has expired. Sign in again to browse Drive.';
  if (err instanceof Error && (err.name === 'TimeoutError' || err.name === 'AbortError')) {
    return 'Google Drive took too long to respond. Try again.';
  }
  return err instanceof Error ? err.message : 'Failed to load folders from Google Drive';
}

function openFolderPicker(): void {
  pickerPath.value = [];
  pickerOpen.value = true;
  void loadPickerFolders();
}

function enterPickerFolder(id: string, name: string): void {
  pickerPath.value = [...pickerPath.value, { id, name }];
  void loadPickerFolders();
}

function goToRoot(): void {
  pickerPath.value = [];
  void loadPickerFolders();
}

function goToCrumb(index: number): void {
  pickerPath.value = pickerPath.value.slice(0, index + 1);
  void loadPickerFolders();
}

function confirmPickerFolder(): void {
  gdriveFolderId.value = currentPickerFolderId.value;
  gdriveCurrentName.value = currentPickerFolderName.value || 'My Drive';
  gdriveError.value = '';
  pickerOpen.value = false;
}

async function loadSources(): Promise<void> {
  savedSources.value = await db.getSources();
}

watch(
  open,
  (isOpen) => {
    if (isOpen) {
      googleToken.value = getGoogleToken();
      gdriveError.value = '';
      loadSources().catch(() => undefined);
    }
  },
  { immediate: true },
);

function useSource(source: SourceRecord): void {
  if (source.type !== 's3' || 'handle' in source.config) return;
  const cfg = source.config as S3Config;
  s3Form.value = { ...cfg, sessionToken: cfg.sessionToken ?? '' };
  sourceType.value = 's3';
}

function rescanSource(source: SourceRecord): void {
  if (source.type === 's3' && !('handle' in source.config)) {
    libraryStore.startScan(source.config as S3Config, source.name);
  } else if (source.type === 'gdrive' && 'folderId' in source.config) {
    const token = getGoogleToken();
    if (token) libraryStore.startScan({ folderId: (source.config as { folderId: string }).folderId, token }, source.name);
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
      // Drop queue entries that belonged to the removed source.
      await playerStore.pruneQueue();
    })();
  });
}

const canScan = computed(() => {
  if (sourceType.value === 'server') return serverFolder.value.trim().length > 0;
  if (sourceType.value === 'local') return !!localHandle.value;
  if (sourceType.value === 'gdrive') return !!googleToken.value && !!gdriveFolderId.value;
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
  } else if (sourceType.value === 'gdrive') {
    const token = getGoogleToken();
    if (!token) return;
    const folderId = gdriveFolderId.value.trim();
    const sourceName = `gdrive-${folderId}`;
    await db.putSource(sourceName, {
      name: sourceName,
      type: 'gdrive' as const,
      config: { folderId, folderName: gdriveCurrentName.value || folderId },
    });
    await loadSources();
    libraryStore.startScan({ folderId, folderName: gdriveCurrentName.value || folderId, token }, sourceName);
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

<style scoped>
.picker-card {
  background: #1a1a2e;
  color: white;
  display: flex;
  flex-direction: column;
  max-width: 560px;
  margin: auto;
  border-radius: 8px;
}

.breadcrumb .q-btn {
  max-width: 180px;
}

.breadcrumb .q-btn :deep(.q-btn__content) {
  text-transform: none;
}
</style>
