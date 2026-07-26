<script lang="ts">
	import { onMount } from 'svelte';
	import { PDFListStore } from '$lib/pdfs.svelte';
	import { apiJSON } from '$lib/api';
	import PdfCard from '$lib/components/pdf-card.svelte';
	import PdfUpload from '$lib/components/pdf-upload.svelte';
	import ScrollSentinel from '$lib/components/scroll-sentinel.svelte';
	import { Button } from '$lib/components/ui/button';
	import type { Layout, PDF } from '$lib/types';

	const list = new PDFListStore();
	let layout = $state<Layout>('grid');

	const LAYOUT_OPTIONS: { value: Layout; icon: string; label: string }[] = [
		{ value: 'grid', icon: 'bx-grid-alt', label: 'Grade' },
		{ value: 'list', icon: 'bx-list-ul', label: 'Lista' },
		{ value: 'compact', icon: 'bx-menu', label: 'Compacto' },
		{ value: 'minimal', icon: 'bx-minus', label: 'Mínimo' }
	];

	onMount(async () => {
		try {
			const settings = await apiJSON<Record<string, string>>('/settings');
			const stored = settings['ui.layout'];
			if (stored === 'grid' || stored === 'list' || stored === 'compact' || stored === 'minimal') {
				layout = stored;
			}
		} catch {
			// defaults stand
		}
		await list.reset();
	});

	async function setLayout(next: Layout) {
		layout = next;
		try {
			await apiJSON('/settings', { method: 'PATCH', body: { 'ui.layout': next } });
		} catch {
			// best-effort persistence
		}
	}

	async function toggleStar(pdf: PDF) {
		const updated = await apiJSON<PDF>(`/pdfs/${pdf.id}`, {
			method: 'PATCH',
			body: { starred: !pdf.starred }
		});
		list.replace(updated);
	}

	function handleUploaded(pdf: PDF) {
		list.prepend(pdf);
	}

	function handleEmbedUpdated(pdf: PDF) {
		list.replace(pdf);
	}
</script>

<div class="mx-auto max-w-6xl p-4">
	<PdfUpload onUploaded={handleUploaded} />

	<div class="mt-4 flex items-center justify-between">
		<h1 class="text-lg font-semibold">Biblioteca</h1>
		<div class="flex gap-1">
			{#each LAYOUT_OPTIONS as opt (opt.value)}
				<Button
					variant={layout === opt.value ? 'secondary' : 'ghost'}
					size="icon"
					onclick={() => setLayout(opt.value)}
					aria-label={opt.label}
				>
					<i class={`bx ${opt.icon}`}></i>
				</Button>
			{/each}
		</div>
	</div>

	{#if list.items.length === 0 && !list.loading}
		<p class="mt-8 text-center text-sm text-muted-foreground">Nenhum PDF ainda. Envie um acima.</p>
	{:else}
		<div
			class={layout === 'grid'
				? 'mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6'
				: 'mt-4 flex flex-col gap-2'}
		>
			{#each list.items as pdf (pdf.id)}
				<PdfCard {pdf} {layout} onStarToggle={toggleStar} onEmbedUpdated={handleEmbedUpdated} />
			{/each}
		</div>
	{/if}

	<ScrollSentinel onIntersect={() => list.loadMore()} disabled={list.done} />
	{#if list.loading}
		<p class="py-4 text-center text-sm text-muted-foreground">Carregando…</p>
	{/if}
</div>
