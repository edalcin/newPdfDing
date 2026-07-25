<script lang="ts">
	import '../app.css';
	import 'boxicons/css/boxicons.min.css';
	import favicon from '$lib/assets/favicon.svg';
	import { onMount } from 'svelte';
	import { auth } from '$lib/auth.svelte';
	import { theme } from '$lib/theme.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import TopBar from '$lib/components/top-bar.svelte';

	let { children } = $props();

	const PUBLIC_ROUTES: Record<string, true> = { '/login': true };

	onMount(async () => {
		const authenticated = await auth.check();
		if (authenticated) await theme.loadFromServer();
		if ('serviceWorker' in navigator) {
			navigator.serviceWorker.register('/sw.js').catch(() => {
				// PWA offline support is best-effort — a failed registration
				// must never block the app from working online.
			});
		}
	});

	$effect(() => {
		if (auth.authenticated === null) return;
		const isPublic = PUBLIC_ROUTES[page.url.pathname] || page.url.pathname.startsWith('/s/');
		if (auth.authenticated && page.url.pathname === '/login') {
			goto('/');
		} else if (!auth.authenticated && !isPublic) {
			goto('/login');
		}
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

{#if auth.authenticated === null}
	<div class="flex h-screen items-center justify-center text-muted-foreground">Carregando…</div>
{:else if PUBLIC_ROUTES[page.url.pathname] || page.url.pathname.startsWith('/s/')}
	{@render children()}
{:else if auth.authenticated}
	<div class="flex min-h-screen flex-col">
		<TopBar />
		<main class="flex-1">
			{@render children()}
		</main>
	</div>
{:else}
	<div class="flex h-screen items-center justify-center text-muted-foreground">Carregando…</div>
{/if}
