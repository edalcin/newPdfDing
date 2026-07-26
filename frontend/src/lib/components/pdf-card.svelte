<script lang="ts">
	import type { Layout, PDF } from '$lib/types';
	import { Button } from '$lib/components/ui/button';
	import EmbedButton from './embed-button.svelte';

	let {
		pdf,
		layout,
		onStarToggle,
		onEmbedUpdated
	}: { pdf: PDF; layout: Layout; onStarToggle: (pdf: PDF) => void; onEmbedUpdated: (pdf: PDF) => void } = $props();

	const thumbnailUrl = $derived(`/api/pdfs/${pdf.id}/thumbnail`);
</script>

{#if layout === 'minimal'}
	<a href={`/pdf/${pdf.id}`} class="flex items-center justify-between border-b border-border px-3 py-2 text-sm hover:bg-accent">
		<span class="truncate">{pdf.name}</span>
		<i class={`bx ${pdf.starred ? 'bxs-star text-yellow-500' : 'bx-star text-muted-foreground'}`}></i>
	</a>
{:else if layout === 'compact'}
	<a href={`/pdf/${pdf.id}`} class="flex items-center gap-3 border-b border-border px-3 py-2 text-sm hover:bg-accent">
		<span class="flex-1 truncate">{pdf.name}</span>
		<span class="text-xs text-muted-foreground">{pdf.num_pages || '—'}p</span>
		<EmbedButton {pdf} onUpdated={onEmbedUpdated} />
		<Button variant="ghost" size="icon" onclick={(e: Event) => { e.preventDefault(); onStarToggle(pdf); }} aria-label="Estrela">
			<i class={`bx ${pdf.starred ? 'bxs-star text-yellow-500' : 'bx-star'}`}></i>
		</Button>
	</a>
{:else if layout === 'list'}
	<a href={`/pdf/${pdf.id}`} class="flex items-center gap-4 rounded-md border border-border p-3 hover:bg-accent">
		<img src={thumbnailUrl} alt="" class="h-16 w-12 rounded object-cover bg-muted" loading="lazy" />
		<div class="min-w-0 flex-1">
			<p class="truncate font-medium">{pdf.name}</p>
			<p class="truncate text-sm text-muted-foreground">{pdf.description}</p>
			<div class="mt-1 flex flex-wrap gap-1">
				{#each pdf.tags as tag (tag.id)}
					<span class="rounded bg-secondary px-1.5 py-0.5 text-xs text-secondary-foreground">{tag.name}</span>
				{/each}
			</div>
		</div>
		<EmbedButton {pdf} onUpdated={onEmbedUpdated} />
		<Button variant="ghost" size="icon" onclick={(e: Event) => { e.preventDefault(); onStarToggle(pdf); }} aria-label="Estrela">
			<i class={`bx ${pdf.starred ? 'bxs-star text-yellow-500' : 'bx-star'}`}></i>
		</Button>
	</a>
{:else}
	<!-- grid (default) -->
	<a href={`/pdf/${pdf.id}`} class="group flex flex-col overflow-hidden rounded-lg border border-border hover:border-primary">
		<div class="relative aspect-[3/4] bg-muted">
			<img src={thumbnailUrl} alt="" class="h-full w-full object-cover" loading="lazy" />
			<button
				type="button"
				class="absolute right-1.5 top-1.5 rounded-full bg-background/80 p-1"
				onclick={(e: Event) => { e.preventDefault(); onStarToggle(pdf); }}
				aria-label="Estrela"
			>
				<i class={`bx ${pdf.starred ? 'bxs-star text-yellow-500' : 'bx-star'}`}></i>
			</button>
		</div>
		<div class="p-2">
			<p class="truncate text-sm font-medium">{pdf.name}</p>
			<div class="mt-1 flex flex-wrap gap-1">
				{#each pdf.tags.slice(0, 3) as tag (tag.id)}
					<span class="rounded bg-secondary px-1.5 py-0.5 text-xs text-secondary-foreground">{tag.name}</span>
				{/each}
			</div>
			<div class="mt-1" onclick={(e: Event) => e.preventDefault()} role="presentation">
				<EmbedButton {pdf} onUpdated={onEmbedUpdated} />
			</div>
		</div>
	</a>
{/if}
