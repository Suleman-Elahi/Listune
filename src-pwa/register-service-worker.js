// Feature: xp-music-player — PWA Service Worker
// Requirements: 9.1, 9.2, 9.3, 9.4

const CACHE_NAME = 'xp-music-player-shell-v1';

const APP_SHELL = [
  '/',
  '/index.html',
];

// Patterns that identify audio stream URLs that must never be cached
function isAudioStreamUrl(url) {
  const urlObj = new URL(url);
  const search = urlObj.search;
  const hostname = urlObj.hostname;

  // Pre-signed S3 URLs contain these query params
  if (search.includes('X-Amz-Signature') || search.includes('X-Amz-Credential')) {
    return true;
  }

  // S3 / S3-compatible object storage hostnames
  if (hostname.includes('.amazonaws.com') || hostname.includes('.s3.')) {
    return true;
  }

  return false;
}

// Install: cache the app shell
self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      return cache.addAll(APP_SHELL);
    })
  );
  self.skipWaiting();
});

// Activate: claim clients immediately
self.addEventListener('activate', (event) => {
  event.waitUntil(
    // Clean up old caches from previous versions
    caches.keys().then((keys) =>
      Promise.all(
        keys
          .filter((key) => key !== CACHE_NAME)
          .map((key) => caches.delete(key))
      )
    )
  );
  self.clients.claim();
});

// Fetch: cache-first for app shell; bypass cache for audio stream URLs
self.addEventListener('fetch', (event) => {
  const { request } = event;

  // Only handle GET requests
  if (request.method !== 'GET') return;

  // Bypass cache entirely for S3 / pre-signed audio URLs
  if (isAudioStreamUrl(request.url)) {
    event.respondWith(fetch(request));
    return;
  }

  // Cache-first strategy for everything else (app shell)
  event.respondWith(
    caches.match(request).then((cached) => {
      if (cached) return cached;

      return fetch(request).then((response) => {
        // Only cache valid responses
        if (!response || response.status !== 200 || response.type === 'opaque') {
          return response;
        }

        const responseToCache = response.clone();
        caches.open(CACHE_NAME).then((cache) => {
          cache.put(request, responseToCache);
        });

        return response;
      });
    })
  );
});
