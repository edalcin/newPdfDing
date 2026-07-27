<script lang="ts">
	import { onMount } from 'svelte';
	import { AnnotationListStore, deleteAnnotation, exportAnnotationsUrl } from '$lib/annotations.svelte';
	import { apiJSON } from '$lib/api';
	import ScrollSentinel from '$lib/components/scroll-sentinel.svelte';
	import { LEGACY_HEX } from '$lib/embedpdf';
	import { formatDate } from '$lib/utils';
	import type { PDF } from '$lib/types';

	const store = new AnnotationListStore();
	store.kind = 'highlight';

	let pdfsById = $state(new Map<string, PDF>());

	onMount(async () => {
		try {
			const page = await apiJSON<{ items: PDF[] }>('/pdfs?limit=200');
			pdfsById = new Map(page.items.map((pdf) => [pdf.id, pdf]));
		} catch {
			// falha na busca de nomes — cai no fallback do pdf_id bruto
		}
		await store.reset();
	});

	async function remove(id: string) {
		if (!confirm('Excluir este destaque?')) return;
		await deleteAnnotation(id);
		store.remove(id);
	}
</script>

<div class="mx-auto max-w-3xl p-4">
	<div class="flex items-center justify-between">
		<h1 class="text-lg font-semibold">Destaques</h1>
		<div class="flex gap-3 text-sm">
			<a class="text-muted-foreground hover:text-foreground" href={exportAnnotationsUrl('highlight', '', 'json')} download>JSON</a>
			<a class="text-muted-foreground hover:text-foreground" href={exportAnnotationsUrl('highlight', '', 'yaml')} download>YAML</a>
		</div>
	</div>

	{#if store.items.length === 0 && !store.loading}
		<p class="mt-8 text-center text-sm text-muted-foreground">Nenhum destaque ainda.</p>
	{:else}
		<ul class="mt-4 flex flex-col gap-2">
			{#each store.items as annotation (annotation.id)}
				{@const pdf = pdfsById.get(annotation.pdf_id)}
				<li class="rounded-md border border-border bg-card p-3">
					<div class="flex items-start gap-2">
						<span
							class="mt-1 h-2.5 w-2.5 shrink-0 rounded-full"
							style:background-color={LEGACY_HEX[annotation.color] ?? annotation.color}
						></span>
						<div class="min-w-0 flex-1">
							<p class="line-clamp-2 text-sm">{annotation.text}</p>
							{#if annotation.note}
								<p class="mt-1 line-clamp-2 text-sm text-muted-foreground italic">{annotation.note}</p>
							{/if}
						</div>
					</div>
					<div class="mt-2 flex items-center justify-between text-xs text-muted-foreground">
						<span>
							<a class="hover:text-foreground hover:underline" href={`/pdf/${annotation.pdf_id}`}>
								{pdf?.name ?? annotation.pdf_id}
							</a>
							&middot; página {annotation.page} &middot; {formatDate(annotation.created_at)}
						</span>
						<button class="text-destructive hover:underline" onclick={() => remove(annotation.id)}>Excluir</button>
					</div>
				</li>
			{/each}
		</ul>
	{/if}

	<ScrollSentinel onIntersect={() => store.loadMore()} disabled={store.done} />
	{#if store.loading}
		<p class="py-4 text-center text-sm text-muted-foreground">Carregando…</p>
	{/if}
</div>
