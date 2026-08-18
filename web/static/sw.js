// ArsipPro PWA Service Worker — Optimized for slow connections
const CACHE_NAME = 'arsipro-mobile-v4';
const OFFLINE_URL = '/mobile/offline';

const STATIC_ASSETS = [
  '/mobile',
  '/manifest.json',
  '/images/logo-icon.svg'
];

const API_CACHE_NAME = 'arsipro-api-v1';
const DATA_CACHE_NAME = 'arsipro-data-v1';

// Limit cache size to prevent quota overflow
const MAX_CACHE_ITEMS = 50;

// Install Event
self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      return cache.addAll(STATIC_ASSETS);
    })
  );
  self.skipWaiting();
});

// Activate Event
self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((cacheNames) => {
      return Promise.all(
        cacheNames.map((cacheName) => {
          if (cacheName !== CACHE_NAME && cacheName !== API_CACHE_NAME && cacheName !== DATA_CACHE_NAME) {
            return caches.delete(cacheName);
          }
        })
      );
    })
  );
  self.clients.claim();
});

// Fetch Event - Network First, Fallback to Cache
self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);

  // Skip chrome-extension:// requests (fix for extension errors)
  if (url.protocol === 'chrome-extension:') {
    return;
  }

  // Skip non-http/https requests
  if (!url.protocol.startsWith('http')) {
    return;
  }

  // API requests - Network First
  if (url.pathname.startsWith('/mobile/api/')) {
    event.respondWith(
      fetch(request)
        .then((response) => {
          const responseClone = response.clone();
          caches.open(API_CACHE_NAME).then((cache) => {
            cache.put(request, responseClone);
          });
          return response;
        })
        .catch(() => {
          return caches.match(request);
        })
    );
    return;
  }

  // Static assets - Network First (CSS always fresh)
  if (STATIC_ASSETS.some(asset => url.pathname.endsWith(asset))) {
    event.respondWith(
      fetch(request)
        .then((response) => {
          const clone = response.clone();
          caches.open(CACHE_NAME).then(cache => cache.put(request, clone));
          return response;
        })
        .catch(() => caches.match(request))
    );
    return;
  }

  // HTML pages - Network First with offline fallback
  if (request.mode === 'navigate') {
    event.respondWith(
      fetch(request)
        .catch(() => {
          return caches.match(OFFLINE_URL);
        })
    );
    return;
  }

  // Default - Network First
  event.respondWith(
    fetch(request)
      .then((response) => {
        const responseClone = response.clone();
        caches.open(DATA_CACHE_NAME).then((cache) => {
          cache.put(request, responseClone);
        });
        return response;
      })
      .catch(() => {
        return caches.match(request);
      })
  );
});

// Background Sync for offline actions
self.addEventListener('sync', (event) => {
  if (event.tag === 'sync-archives') {
    event.waitUntil(syncArchives());
  }
});

async function syncArchives() {
  // Get pending requests from IndexedDB
  // Send to server when online
  console.log('Syncing archives...');
}

// Push Notifications
self.addEventListener('push', (event) => {
  const data = event.data ? event.data.json() : {};
  
  const options = {
    body: data.body || 'Notifikasi baru dari ArsipPro',
    icon: '/images/logo-icon.svg',
    badge: '/images/logo-icon.svg',
    vibrate: [100, 50, 100],
    data: {
      url: data.url || '/mobile'
    }
  };

  event.waitUntil(
    self.registration.showNotification(data.title || 'ArsipPro', options)
  );
});

// Notification Click
self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  
  event.waitUntil(
    clients.openWindow(event.notification.data.url)
  );
});
