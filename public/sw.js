// Listune Service Worker
const CACHE_NAME = 'listune-shell-v2';

// Patterns that identify audio stream URLs that must never be cached
function isAudioStreamUrl(url) {
  const search = new URL(url).search;
  if (search.includes('X-Amz-Signature') || search.includes('X-Amz-Credential')) return true;
  if (url.includes('/api/s3proxy')) return true;
  if (url.includes('/api/stream')) return true;
  return false;
}

function isCacheableRequest(request) {
  if (request.method !== 'GET') return false;

  let url;
  try {
    url = new URL(request.url);
  } catch {
    return false;
  }

  // Cache.put() only supports http(s) requests from this origin. This avoids
  // errors for chrome-extension://, devtools:// and other schemes.
  if (url.protocol !== 'http:' && url.protocol !== 'https:') return false;
  if (url.origin !== self.location.origin) return false;
  if (url.pathname === '/sw.js') return false;

  // API/auth responses are user-specific or signed; never cache them.
  if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/auth/')) return false;
  if (request.headers.has('range')) return false;
  if (isAudioStreamUrl(request.url)) return false;

  return true;
}

self.addEventListener('install', () => {
  void self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k)))
    )
  );
  void self.clients.claim();
});

self.addEventListener('fetch', (event) => {
  const { request } = event;
  if (!isCacheableRequest(request)) return;

  // Cache-first for app shell assets
  event.respondWith(
    caches.match(request).then((cached) => {
      if (cached) return cached;
      return fetch(request).then((response) => {
        if (!response || response.status !== 200 || response.type === 'opaque') return response;
        const clone = response.clone();
        caches
          .open(CACHE_NAME)
          .then((cache) => cache.put(request, clone))
          .catch(() => undefined);
        return response;
      });
    })
  );
});
