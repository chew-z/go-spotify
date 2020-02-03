// Service Worker
console.log('Service Worker [registerd**]');

const urlsToCache = [
    '/static/index.js',
    '/static/music.svg',
    '/static/share_button.svg',
    '/static/generic_album_cover.png',
    '/static/favicon-32x32.png',
    '/static/android-icon-192x192.png',
    '/static/android-icon-512x512.png',
];
const CACHE_NAME = 'go-spotify-v1';

const cacheResources = async () => {
    const cache = await caches.open(CACHE_NAME);

    return cache.addAll(urlsToCache);
};

self.addEventListener('install', async (e) => {
    self.skipWaiting();
    e.waitUntil(cacheResources());
});

const clearOldCache = async () => {
    const cacheNames = await caches.keys();
    const oldCacheName = cacheNames.find((name) => name !== CACHE_NAME);
    // Feature-detect
    if (self.registration.navigationPreload) {
        // Enable navigation preloads!
        await self.registration.navigationPreload.enable();
    }
    caches.delete(oldCacheName);
};

self.addEventListener('activate', (e) => {
    e.waitUntil(clearOldCache());
});

const getResponseByRequest = async (e) => {
    const cache = await caches.open(CACHE_NAME);
    const cachedResponse = await cache.match(e.request);
    // Else, use the preloaded response, if it's there
    const preloadedResponse = await e.preloadResponse;

    return cachedResponse || preloadedResponse || fetch(e.request);
};

self.addEventListener('fetch', (e) => {
    console.log(e.request.url);
    e.respondWith(getResponseByRequest(e));
});
