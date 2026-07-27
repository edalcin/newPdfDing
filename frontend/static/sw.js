// Service worker — estratégias de cache fixadas em
// refatoracao/06-frontend.md, "PWA e service worker". Dois caches
// versionados: SHELL_CACHE (app + assets estáticos) e API_CACHE (fallback
// offline de GET /api/pdfs), invalidáveis independentemente a cada deploy.
//
// VERSION é substituída em build time por scripts/stamp-sw-version.mjs com
// um hash do conteúdo real de frontend/build/ — nunca editar à mão. Um
// valor fixo aqui já causou cache desatualizado após deploys que mudaram
// viewer.mjs/viewer.html sem ninguém lembrar de bumpar a string.
const VERSION = '__BUILD_VERSION__';
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
		url.pathname.startsWith('/embedpdf/') ||
		/\.(?:js|css|woff2?|png|svg|ico)$/.test(url.pathname)
	);
}

// fetchWithTimeout guards every network attempt below: a stalled connection
// (or a fetch that never settles for any reason) must fail over to cache —
// or to the offline response for mutations — instead of hanging the page
// forever. AbortController.abort() rejects the fetch even if it's stuck at
// the browser/network layer, not just on a genuine slow response.
const NETWORK_TIMEOUT_MS = 8000;

async function fetchWithTimeout(request, { bypassHTTPCache = false } = {}) {
	const controller = new AbortController();
	const timer = setTimeout(() => controller.abort(), NETWORK_TIMEOUT_MS);
	try {
		// bypassHTTPCache: index.html references content-hashed asset paths
		// that change on every build — a browser-cached copy of the shell can
		// point at chunks the current deploy no longer serves, breaking
		// client-side navigation. 'no-store' forces this fetch past the
		// browser's own HTTP cache, not just past this service worker's Cache
		// Storage (which networkFirst already treats as fallback-only).
		const init = { signal: controller.signal };
		if (bypassHTTPCache) init.cache = 'no-store';
		return await fetch(request, init);
	} finally {
		clearTimeout(timer);
	}
}

async function networkFirst(request, cacheName, options) {
	const cache = await caches.open(cacheName);
	try {
		const response = await fetchWithTimeout(request, options);
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
	const response = await fetchWithTimeout(request);
	if (response.ok) cache.put(request, response.clone());
	return response;
}

self.addEventListener('fetch', (event) => {
	const { request } = event;
	const url = new URL(request.url);

	if (url.origin !== self.location.origin) return;

	// Mutações: network-only, 503 explícito quando offline ou a rede trava
	// (ver 06-frontend.md).
	if (isMutation(request)) {
		event.respondWith(
			fetchWithTimeout(request).catch(
				() => new Response(JSON.stringify({ error: 'offline' }), { status: 503, headers: { 'Content-Type': 'application/json' } })
			)
		);
		return;
	}

	if (url.pathname === '/' || url.pathname === '/index.html') {
		event.respondWith(networkFirst(request, SHELL_CACHE, { bypassHTTPCache: true }));
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
