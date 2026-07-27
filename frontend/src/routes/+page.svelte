<script lang="ts">
	import { onMount } from 'svelte';
	import { PDFListStore } from '$lib/pdfs.svelte';
	import { apiJSON } from '$lib/api';
	import PdfCard from '$lib/components/pdf-card.svelte';
	import PdfUpload from '$lib/components/pdf-upload.svelte';
	import ScrollSentinel from '$lib/components/scroll-sentinel.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import type { Layout, PDF, TagWithCount } from '$lib/types';

	const list = new PDFListStore();
	let layout = $state<Layout>('grid');
	let tags = $state<TagWithCount[]>([]);
	let searchInput = $state('');
	let searchTimer: ReturnType<typeof setTimeout> | undefined;

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
		try {
			tags = await apiJSON<TagWithCount[]>('/tags');
		} catch {
			// biblioteca continua funcionando sem a lista de tags
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

	function handleSearchInput(value: string) {
		searchInput = value;
		clearTimeout(searchTimer);
		searchTimer = setTimeout(() => {
			list.query = value.trim();
			list.reset();
		}, 300);
	}

	function selectTag(name: string) {
		list.tags = list.tags.includes(name) ? list.tags.filter((t) => t !== name) : [...list.tags, name];
		list.reset();
	}

	function clearTagFilters() {
		list.tags = [];
		list.reset();
	}

	function toggleArchivedView() {
		list.archived = !list.archived;
		list.reset();
	}

	async function unarchivePdf(pdf: PDF) {
		await apiJSON<PDF>(`/pdfs/${pdf.id}`, { method: 'PATCH', body: { archived: false } });
		list.remove(pdf.id);
	}

	async function deletePdf(pdf: PDF) {
		if (!confirm(`Excluir "${pdf.name}"? Esta ação não pode ser desfeita.`)) return;
		await apiJSON(`/pdfs/${pdf.id}`, { method: 'DELETE' });
		list.remove(pdf.id);
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
		<h1 class="text-lg font-semibold">{list.archived ? 'Arquivados' : 'Biblioteca'}</h1>
		<div class="flex gap-1">
			<Button
				variant={list.archived ? 'secondary' : 'ghost'}
				size="icon"
				onclick={toggleArchivedView}
				aria-label="Arquivados"
			>
				<i class="bx bx-archive"></i>
			</Button>
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

	<div class="relative mt-3">
		<Input
			value={searchInput}
			oninput={(e) => handleSearchInput((e.target as HTMLInputElement).value)}
			placeholder="Buscar por nome, descrição, conteúdo…"
			aria-label="Buscar PDFs"
			class={searchInput ? 'pr-8' : ''}
		/>
		{#if searchInput}
			<button
				type="button"
				onclick={() => handleSearchInput('')}
				aria-label="Limpar busca"
				class="text-muted-foreground hover:text-foreground absolute inset-y-0 right-2 flex items-center"
			>
				<i class="bx bx-x text-lg"></i>
			</button>
		{/if}
	</div>

	{#if tags.length > 0}
		<div class="mt-2 flex flex-wrap gap-1.5">
			{#each tags as tag (tag.id)}
				<button
					type="button"
					onclick={() => selectTag(tag.name)}
					class={`rounded-full border px-2.5 py-0.5 text-xs transition-colors ${
						list.tags.includes(tag.name)
							? 'border-primary bg-primary text-primary-foreground'
							: 'border-border bg-background text-muted-foreground hover:bg-accent hover:text-accent-foreground'
					}`}
				>
					{tag.name} <span class="opacity-70">{tag.count}</span>
				</button>
			{/each}
			{#if list.tags.length > 0}
				<button
					type="button"
					onclick={clearTagFilters}
					class="rounded-full border border-border bg-background px-2.5 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
				>
					<i class="bx bx-x"></i> Limpar filtros
				</button>
			{/if}
		</div>
	{/if}

	{#if list.items.length === 0 && !list.loading}
		<p class="mt-8 text-center text-sm text-muted-foreground">
			{list.query || list.tags.length > 0 ? 'Nenhum PDF encontrado.' : 'Nenhum PDF ainda. Envie um acima.'}
		</p>
	{:else}
		<div
			class={layout === 'grid'
				? 'mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6'
				: 'mt-4 flex flex-col gap-2'}
		>
			{#each list.items as pdf (pdf.id)}
				<PdfCard {pdf} {layout} onStarToggle={toggleStar} onEmbedUpdated={handleEmbedUpdated} onUnarchive={unarchivePdf} onDelete={deletePdf} />
			{/each}
		</div>
	{/if}

	<ScrollSentinel onIntersect={() => list.loadMore()} disabled={list.done} />
	{#if list.loading}
		<p class="py-4 text-center text-sm text-muted-foreground">Carregando…</p>
	{/if}
</div>
