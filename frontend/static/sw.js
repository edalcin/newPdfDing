// Service worker — estratégias de cache fixadas em
// refatoracao/06-frontend.md, "PWA e service worker". Dois caches
// versionados: SHELL_CACHE (app + assets estáticos) e API_CACHE (fallback
// offline de GET /api/pdfs), invalidáveis independentemente a cada deploy.

const VERSION = 'v1';
const SHELL_CACHE = `newpdfding-shell-${VERSION}`;
const API_CACHE = `newpdfding-api-${VERSION}`;

self.addEventListener('install', (event) => {
	self.skipWaiting();
});

self.addEventListener('activate', (event) => {
	event.waitUntil(
		caches.keys().then((keys) =>
			Promise.all(
				keys
					.filter((key) => key !== SHELL_CACHE && key !== API_CACHE)
					.map((key) => caches.delete(key))
			)
		)
	);
	self.clients.claim();
});

function isMutation(request) {
	return request.method !== 'GET' && request.method !== 'HEAD';
}

function isStaticAsset(url) {
	return (
		url.pathname.startsWith('/_app/') ||
		url.pathname.startsWith('/icons/') ||
		url.pathname.startsWith('/pdfjs/') ||
		/\.(?:js|css|woff2?|png|svg|ico)$/.test(url.pathname)
	);
}

async function networkFirst(request, cacheName) {
	const cache = await caches.open(cacheName);
	try {
		const response = await fetch(request);
		if (response.ok) cache.put(request, response.clone());
		return response;
	} catch {
		const cached = await cache.match(request);
		if (cached) return cached;
		throw new Error('offline and no cache entry');
	}
}

async function cacheFirst(request, cacheName) {
	const cache = await caches.open(cacheName);
	const cached = await cache.match(request);
	if (cached) return cached;
	const response = await fetch(request);
	if (response.ok) cache.put(request, response.clone());
	return response;
}

self.addEventListener('fetch', (event) => {
	const { request } = event;
	const url = new URL(request.url);

	if (url.origin !== self.location.origin) return;

	// Mutações: network-only, 503 explícito quando offline (ver 06-frontend.md).
	if (isMutation(request)) {
		event.respondWith(
			fetch(request).catch(
				() => new Response(JSON.stringify({ error: 'offline' }), { status: 503, headers: { 'Content-Type': 'application/json' } })
			)
		);
		return;
	}

	if (url.pathname === '/' || url.pathname === '/index.html') {
		event.respondWith(networkFirst(request, SHELL_CACHE));
		return;
	}

	if (url.pathname === '/api/pdfs') {
		event.respondWith(networkFirst(request, API_CACHE));
		return;
	}

	if (isStaticAsset(url)) {
		event.respondWith(cacheFirst(request, SHELL_CACHE));
		return;
	}

	// Demais rotas da SPA e chamadas de API: passam direto pela rede.
});
