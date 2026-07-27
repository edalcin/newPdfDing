<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { apiJSON, ApiError } from '$lib/api';
	import PdfViewer from '$lib/components/pdf-viewer.svelte';
	import type { PDF } from '$lib/types';

	const shareId = page.params.share;

	let pdf = $state<PDF | null>(null);
	let notFound = $state(false);
	let loadError = $state('');

	onMount(() => {
		(async () => {
			try {
				pdf = await apiJSON<PDF>(`/shared/${shareId}`);
			} catch (err) {
				if (err instanceof ApiError && err.status === 404) {
					notFound = true;
				} else {
					loadError = err instanceof Error ? err.message : 'Falha ao carregar o PDF compartilhado.';
				}
			}
		})();
	});

	const fileUrl = $derived(`/api/shared/${shareId}/file`);
</script>

<div class="flex min-h-screen flex-col items-center bg-background text-foreground">
	{#if notFound}
		<p class="mt-16 text-center text-sm text-muted-foreground">
			Este link de compartilhamento não existe ou foi revogado.
		</p>
	{:else if loadError}
		<p class="mt-16 text-center text-sm text-destructive">{loadError}</p>
	{:else if pdf}
		<header class="w-full max-w-4xl border-b border-border p-4">
			<h1 class="truncate text-lg font-semibold">{pdf.name}</h1>
			{#if pdf.description}
				<p class="mt-1 text-sm text-muted-foreground">{pdf.description}</p>
			{/if}
		</header>

		<div class="mt-2 w-full max-w-4xl flex-1 px-4 pb-4">
			<div class="h-[85vh] w-full overflow-hidden rounded-md border border-border">
				<PdfViewer src={fileUrl} readonly />
			</div>
		</div>
	{:else}
		<p class="mt-16 text-center text-sm text-muted-foreground">Carregando…</p>
	{/if}
</div>
