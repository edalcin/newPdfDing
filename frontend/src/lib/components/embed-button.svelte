<script lang="ts">
	// Botão de embedding assíncrono — o clique só enfileira o job (202); o
	// rótulo e o ícone acompanham o estado do job pelo mapa global da store
	// (ver refatoracao Fase F), que a página mantém atualizado por polling.
	// O botão não observa nada por conta própria: a atualização do PDF ao
	// final do job é responsabilidade de quem exibe a lista, para que ela
	// aconteça mesmo se este botão nunca tiver sido clicado nesta página.
	import { apiJSON, ApiError } from '$lib/api';
	import { embedSession } from '$lib/embed.svelte';
	import { embedJobs } from '$lib/embed-jobs.svelte';
	import type { PDF } from '$lib/types';

	let { pdf, showLabel = false }: { pdf: PDF; showLabel?: boolean } = $props();

	let toast = $state('');

	const STATUS_LABELS: Record<PDF['embedding_status'], string> = {
		none: 'Embedar',
		current: 'Embedado',
		stale: 'Reembedar'
	};

	const JOB_LABELS: Record<string, string> = {
		queued: 'Na fila…',
		extracting: 'Extraindo texto…',
		embedding: 'Embedando…'
	};

	const ICON_CLASS: Record<PDF['embedding_status'], string> = {
		none: 'bx-brain text-muted-foreground/40',
		current: 'bxs-brain text-primary',
		stale: 'bxs-brain text-amber-500'
	};

	const ICON_TITLE: Record<PDF['embedding_status'], string> = {
		none: 'Sem embedding (cérebro vazio com ponto vermelho) — clique para gerar e habilitar a busca semântica.',
		current: 'Embedding atualizado (cérebro preenchido) — este PDF já entra na busca semântica.',
		stale: 'Embedding desatualizado (cérebro em âmbar) — o conteúdo mudou; clique para reembedar.'
	};

	const job = $derived(embedJobs.jobs[pdf.id]);
	const busy = $derived(!!job && job.state !== 'failed');
	const clickable = $derived(!busy && !embedSession.disabledGlobally && pdf.embedding_status !== 'current');
	const label = $derived(
		job?.state === 'failed' ? job.error || 'Falha ao embedar' : busy ? JOB_LABELS[job!.state] : STATUS_LABELS[pdf.embedding_status]
	);
	const tooltip = $derived(job?.state === 'failed' ? job.error || '' : ICON_TITLE[pdf.embedding_status]);

	async function handleClick(e: Event) {
		e.preventDefault();
		if (!clickable) return;
		toast = '';
		try {
			await apiJSON<{ state: string }>(`/pdfs/${pdf.id}/embed`, { method: 'POST' });
			await embedJobs.poll();
		} catch (err) {
			if (err instanceof ApiError && err.status === 412) {
				embedSession.disabledGlobally = true;
				toast = 'Busca semântica não está configurada (GEMINI_API_KEY ausente).';
			} else if (err instanceof ApiError && err.status === 503) {
				toast = 'Fila de embedding cheia — tente novamente em instantes.';
			} else if (err instanceof ApiError && err.status !== 409 && err.status !== 404) {
				toast = err.message;
			}
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
	<span class="relative inline-flex">
		<i class={`bx ${ICON_CLASS[pdf.embedding_status]} text-base ${busy ? 'animate-pulse' : ''}`}></i>
		{#if pdf.embedding_status === 'none'}
			<span class="absolute -right-0.5 -top-0.5 h-1.5 w-1.5 rounded-full bg-destructive"></span>
		{/if}
	</span>
	{#if showLabel}<span>{label}</span>{/if}
</button>
{#if job?.state === 'failed'}
	<p class="text-xs text-destructive">{job.error}</p>
{:else if toast}
	<p class="text-xs text-destructive">{toast}</p>
{/if}
