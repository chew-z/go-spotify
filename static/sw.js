// Service Worker
console.log('Service Worker [registerd**]');

const cacheName = 'go-spotify';
const filesToCache = ['/', '/static/index.js', '/static/share_button.svg', '/static/generic_album_cover.png'];

self.addEventListener('install', (e) => {
    console.log('[ServiceWorker**] Install');
    e.waitUntil(
        caches.open(cacheName).then((cache) => {
            console.log('[ServiceWorker**] Caching app shell');

            return cache.addAll(filesToCache);
        }),
    );
});

self.addEventListener('activate', (event) => {
    caches.keys().then((keyList) => {
        return Promise.all(
            keyList.map((key) => {
                if (key !== cacheName) {
                    console.log('[ServiceWorker] Removing old cache', key);

                    return caches.delete(key);
                }
            }),
        );
    });
});

self.addEventListener('fetch', (event) => {
    console.log(event.request.url);
    event.respondWith(
        caches.match(event.request, { ignoreSearch: true }).then((response) => {
            return response || fetch(event.request);
        }),
    );
});
