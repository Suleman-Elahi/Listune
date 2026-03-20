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
          <q-tab name="local" label="Local Folder" icon="folder" />
          <q-tab name="s3" label="S3 / Cloud" icon="cloud" />
        </q-tabs>

        <q-tab-panels v-model="sourceType" animated style="background: transparent">
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
import type { S3Config, LocalSource } from 'src/types/models';

const open = defineModel<boolean>({ default: false });

const libraryStore = useLibraryStore();
const sourceType = ref<'local' | 's3'>('local');
const showSecret = ref(false);

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
  if (sourceType.value === 'local') return !!localHandle.value;
  const f = s3Form.value;
  return !!(f.endpoint && f.bucket && f.region && f.accessKey && f.secretKey);
});

function startScan() {
  if (sourceType.value === 'local' && localHandle.value) {
    const source: LocalSource = { handle: localHandle.value };
    // Save the handle to DB so playback can re-open files later
    void db.putSource(localHandle.value.name, {
      name: localHandle.value.name,
      type: 'local',
      config: { handle: localHandle.value },
    });
    libraryStore.startScan(source);
  } else if (sourceType.value === 's3') {
    libraryStore.startScan({ ...s3Form.value });
  }
  open.value = false;
}
</script>
