// Listune Service Worker
const CACHE_NAME = 'listune-shell-v1';

// Patterns that identify audio stream URLs that must never be cached
function isAudioStreamUrl(url) {
  const search = new URL(url).search;
  if (search.includes('X-Amz-Signature') || search.includes('X-Amz-Credential')) return true;
  if (url.includes('/api/s3proxy')) return true;
  if (url.includes('/api/stream')) return true;
  return false;
}

self.addEventListener('install', (event) => {
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k)))
    )
  );
  self.clients.claim();
});

self.addEventListener('fetch', (event) => {
  const { request } = event;
  if (request.method !== 'GET') return;

  // Never cache audio streams
  if (isAudioStreamUrl(request.url)) {
    event.respondWith(fetch(request));
    return;
  }

  // Cache-first for app shell assets
  event.respondWith(
    caches.match(request).then((cached) => {
      if (cached) return cached;
      return fetch(request).then((response) => {
        if (!response || response.status !== 200 || response.type === 'opaque') return response;
        const clone = response.clone();
        caches.open(CACHE_NAME).then((cache) => cache.put(request, clone));
        return response;
      });
    })
  );
});
