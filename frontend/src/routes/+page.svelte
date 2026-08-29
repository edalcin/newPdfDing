<script lang="ts">
	import { onMount } from 'svelte';
	import { PDFListStore } from '$lib/pdfs.svelte';
	import { apiJSON } from '$lib/api';
	import { embedJobs } from '$lib/embed-jobs.svelte';
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

	// Os três estados que o servidor deriva (ver semantic.go,
	// attachEmbeddingStatus); clicar no filtro ativo o desliga.
	const EMBEDDING_FILTERS: {
		value: 'none' | 'current' | 'stale';
		label: string;
		icon: string;
		iconColor: string;
		title: string;
	}[] = [
		{
			value: 'none',
			label: 'Sem embedding',
			icon: 'bx-brain',
			iconColor: 'text-muted-foreground/60',
			title: 'Mostrar apenas os PDFs que ainda não têm embedding'
		},
		{
			value: 'current',
			label: 'Atualizado',
			icon: 'bxs-brain',
			iconColor: 'text-primary',
			title: 'Mostrar apenas os PDFs com embedding atualizado'
		},
		{
			value: 'stale',
			label: 'Desatualizado',
			icon: 'bxs-brain',
			iconColor: 'text-amber-500',
			title: 'Mostrar apenas os PDFs cujo conteúdo mudou desde o último embedding'
		}
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

	function toggleStarredFilter() {
		list.starred = !list.starred;
		list.reset();
	}

	function selectEmbedding(value: 'none' | 'current' | 'stale') {
		list.embedding = list.embedding === value ? '' : value;
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
		if (list.starred && !updated.starred) list.remove(updated.id);
		else list.replace(updated);
	}

	function handleUploaded(pdf: PDF) {
		list.prepend(pdf);
	}

	// Enquanto esta página estiver montada, qualquer job que termine atualiza
	// o card correspondente — inclusive um job enfileirado antes de a página
	// carregar, em outra aba ou em outro dispositivo. Sem isso, o ícone só
	// mudava depois de recarregar a página.
	$effect(() =>
		embedJobs.onSettled(async (pdfId) => {
			if (!list.items.some((item) => item.id === pdfId)) return;
			try {
				list.replace(await apiJSON<PDF>(`/pdfs/${pdfId}`));
			} catch {
				// melhor-esforço — o card mantém o estado anterior
			}
		})
	);
</script>

<div class="mx-auto max-w-6xl p-4">
	<PdfUpload onUploaded={handleUploaded} />

	<div class="mt-4 flex items-center justify-between">
		<h1 class="text-lg font-semibold">{list.archived ? 'Arquivados' : 'Biblioteca'}</h1>
		<div class="flex gap-1">
			<Button
				variant={list.starred ? 'secondary' : 'ghost'}
				size="icon"
				onclick={toggleStarredFilter}
				aria-label="Apenas com estrela"
				title="Mostrar apenas os PDFs marcados com estrela"
			>
				<i class={`bx ${list.starred ? 'bxs-star text-yellow-500' : 'bx-star'}`}></i>
			</Button>
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

	<div class="mt-2 flex flex-wrap items-center gap-1.5">
		<span class="text-xs text-muted-foreground">Embedding:</span>
		{#each EMBEDDING_FILTERS as opt (opt.value)}
			<button
				type="button"
				onclick={() => selectEmbedding(opt.value)}
				title={opt.title}
				aria-pressed={list.embedding === opt.value}
				class={`inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs transition-colors ${
					list.embedding === opt.value
						? 'border-primary bg-primary text-primary-foreground'
						: 'border-border bg-background text-muted-foreground hover:bg-accent hover:text-accent-foreground'
				}`}
			>
				<i class={`bx ${opt.icon} ${list.embedding === opt.value ? '' : opt.iconColor}`}></i>
				{opt.label}
			</button>
		{/each}
	</div>

	<details class="mt-3 text-xs text-muted-foreground">
		<summary class="cursor-pointer select-none">Legenda dos ícones de embedding</summary>
		<ul class="mt-2 space-y-1">
			<li class="flex items-center gap-2">
				<span class="relative inline-flex">
					<i class="bx bx-brain text-base text-muted-foreground/40"></i>
					<span class="absolute -right-0.5 -top-0.5 h-1.5 w-1.5 rounded-full bg-destructive"></span>
				</span>
				Sem embedding — clique no ícone para gerar e habilitar a busca semântica.
			</li>
			<li class="flex items-center gap-2">
				<i class="bx bxs-brain text-base text-primary"></i>
				Embedding atualizado — o PDF já entra na busca semântica.
			</li>
			<li class="flex items-center gap-2">
				<i class="bx bxs-brain text-base text-amber-500"></i>
				Embedding desatualizado — o conteúdo mudou desde o último embedding; clique para reembedar.
			</li>
		</ul>
	</details>

	{#if list.items.length === 0 && !list.loading}
		<p class="mt-8 text-center text-sm text-muted-foreground">
			{list.query || list.tags.length > 0 || list.starred || list.embedding
				? 'Nenhum PDF encontrado.'
				: 'Nenhum PDF ainda. Envie um acima.'}
		</p>
	{:else}
		<div
			class={layout === 'grid'
				? 'mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6'
				: 'mt-4 flex flex-col gap-2'}
		>
			{#each list.items as pdf (pdf.id)}
				<PdfCard {pdf} {layout} onStarToggle={toggleStar} onUnarchive={unarchivePdf} onDelete={deletePdf} />
			{/each}
		</div>
	{/if}

	<ScrollSentinel onIntersect={() => list.loadMore()} disabled={list.done} />
	{#if list.loading}
		<p class="py-4 text-center text-sm text-muted-foreground">Carregando…</p>
	{/if}
</div>
