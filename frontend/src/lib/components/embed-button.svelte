<script lang="ts">
	// Botão de embedding sob demanda — máquina de estados fixada em
	// refatoracao/06-frontend.md, "Botão de embedding — máquina de estados".
	// O estado é dirigido EXCLUSIVAMENTE por pdf.embedding_status, nunca por
	// estado local inferido.
	import { apiJSON, ApiError } from '$lib/api';
	import { embedSession } from '$lib/embed.svelte';
	import type { PDF } from '$lib/types';

	let {
		pdf,
		onUpdated,
		showLabel = false
	}: { pdf: PDF; onUpdated: (pdf: PDF) => void; showLabel?: boolean } = $props();

	let loading = $state(false);
	let toast = $state('');

	const LABELS: Record<PDF['embedding_status'], string> = {
		none: 'Embedar',
		current: 'Embedado',
		stale: 'Reembedar'
	};

	const clickable = $derived(!loading && !embedSession.disabledGlobally && pdf.embedding_status !== 'current');
	const label = $derived(loading ? 'Embedando…' : LABELS[pdf.embedding_status]);
	const tooltip = $derived(pdf.embedding_status === 'stale' ? 'o conteúdo mudou desde o último embedding' : '');

	async function handleClick(e: Event) {
		e.preventDefault();
		if (!clickable) return;
		loading = true;
		toast = '';
		try {
			await apiJSON<{ embedding_status: string; dimensions: number }>(`/pdfs/${pdf.id}/embed`, {
				method: 'POST'
			});
			onUpdated({ ...pdf, embedding_status: 'current' });
		} catch (err) {
			if (err instanceof ApiError && err.status === 412) {
				embedSession.disabledGlobally = true;
				toast = 'Busca semântica não está configurada (GEMINI_API_KEY ausente).';
			} else if (err instanceof ApiError && (err.status === 422 || err.status === 502)) {
				toast = err.message;
			} else if (err instanceof ApiError && err.status !== 409 && err.status !== 404) {
				toast = err.message;
			}
		} finally {
			loading = false;
		}
	}
</script>

<button
	type="button"
	class="inline-flex items-center gap-1 rounded-md px-1.5 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50"
	disabled={!clickable}
	title={tooltip}
	aria-label={label}
	onclick={handleClick}
>
	<i class={`bx bx-brain ${loading ? 'animate-pulse' : ''}`}></i>
	{#if showLabel}<span>{label}</span>{/if}
</button>
{#if toast}
	<p class="text-xs text-destructive">{toast}</p>
{/if}
