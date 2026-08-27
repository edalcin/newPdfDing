<script lang="ts">
	import { onMount } from 'svelte';
	import { apiJSON, ApiError } from '$lib/api';
	import { formatBytes, formatDate } from '$lib/utils';
	import { Button, buttonVariants } from '$lib/components/ui/button';
	import type { AdminInfo, Share } from '$lib/types';

	let info = $state<AdminInfo | null>(null);
	let shares = $state<Share[]>([]);
	let loading = $state(true);
	let loadError = $state('');
	let reindexing = $state(false);
	let reindexMessage = $state('');
	let revoking = $state<string | null>(null);
	let copiedId = $state<string | null>(null);
	let restoreInput = $state<HTMLInputElement>();
	let restoring = $state(false);
	let restoreMessage = $state('');
	let restoreError = $state('');

	onMount(async () => {
		try {
			[info, shares] = await Promise.all([
				apiJSON<AdminInfo>('/admin/info'),
				apiJSON<Share[]>('/shares')
			]);
		} catch (err) {
			loadError = err instanceof Error ? err.message : 'Falha ao carregar informações.';
		} finally {
			loading = false;
		}
	});

	async function reindex() {
		reindexing = true;
		reindexMessage = '';
		try {
			const res = await apiJSON<{ reindexed: boolean }>('/admin/reindex', { method: 'POST' });
			if (res.reindexed) reindexMessage = 'Índice reconstruído.';
		} catch (err) {
			reindexMessage = err instanceof Error ? err.message : 'Falha ao reindexar.';
		} finally {
			reindexing = false;
		}
	}

	function shareUrl(id: string): string {
		return `${location.origin}/s/${id}`;
	}

	async function copyLink(id: string) {
		await navigator.clipboard.writeText(shareUrl(id));
		copiedId = id;
		setTimeout(() => {
			if (copiedId === id) copiedId = null;
		}, 1500);
	}

	async function revoke(share: Share) {
		if (!confirm(`Revogar o compartilhamento de "${share.pdf_name}"?`)) return;
		revoking = share.id;
		try {
			await apiJSON(`/pdfs/${share.pdf_id}/share`, { method: 'DELETE' });
			shares = shares.filter((s) => s.id !== share.id);
		} catch (err) {
			loadError = err instanceof Error ? err.message : 'Falha ao revogar compartilhamento.';
		} finally {
			revoking = null;
		}
	}

	async function handleRestoreFile(e: Event) {
		const file = (e.target as HTMLInputElement).files?.[0];
		if (!file) return;
		if (restoreInput) restoreInput.value = '';
		if (
			!confirm(
				`Restaurar "${file.name}" substitui TODOS os dados atuais (PDFs, tags, anotações, configurações) pelo conteúdo desse backup e reinicia o servidor. Esta ação não pode ser desfeita. Continuar?`
			)
		)
			return;

		restoring = true;
		restoreError = '';
		restoreMessage = '';
		try {
			const res = await apiJSON<{ restored: boolean; restarting: boolean }>('/admin/restore', {
				method: 'POST',
				body: file
			});
			if (res.restored) {
				restoreMessage = 'Backup restaurado. Reiniciando o servidor…';
				setTimeout(() => location.reload(), 3000);
			}
		} catch (err) {
			restoreError =
				err instanceof ApiError ? err.message : err instanceof Error ? err.message : 'Falha ao restaurar backup.';
			restoring = false;
		}
	}
</script>

<div class="mx-auto max-w-3xl p-4">
	<h1 class="text-lg font-semibold">Administração</h1>

	{#if loading}
		<p class="mt-8 text-center text-sm text-muted-foreground">Carregando…</p>
	{:else if loadError && !info}
		<p class="mt-8 text-center text-sm text-destructive">{loadError}</p>
	{:else if info}
		<section class="mt-4 rounded-lg border border-border bg-card p-4">
			<div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
				<div class="rounded-md border border-border p-3 text-center">
					<p class="text-lg font-semibold">{info.pdfs_count}</p>
					<p class="text-xs text-muted-foreground">PDFs</p>
				</div>
				<div class="rounded-md border border-border p-3 text-center">
					<p class="text-lg font-semibold">{info.tags_count}</p>
					<p class="text-xs text-muted-foreground">Tags</p>
				</div>
				<div class="rounded-md border border-border p-3 text-center">
					<p class="text-lg font-semibold">{info.embedding_status_counts.current + info.embedding_status_counts.stale}</p>
					<p class="text-xs text-muted-foreground">Embedados</p>
				</div>
				<div class="rounded-md border border-border p-3 text-center">
					<p class="text-lg font-semibold">{formatBytes(info.files_bytes)}</p>
					<p class="text-xs text-muted-foreground">Armazenamento</p>
				</div>
			</div>

			<p class="mt-3 text-xs text-muted-foreground">
				Modelo de embedding (fixo): <span class="font-medium text-foreground">{info.embed_model}</span>
			</p>

			<h3 class="mt-4 text-xs font-medium text-muted-foreground">Status de embedding</h3>
			<div class="mt-2 grid grid-cols-3 gap-3">
				<div class="rounded-md border border-border p-2 text-center">
					<p class="text-sm font-semibold">{info.embedding_status_counts.none}</p>
					<p class="text-xs text-muted-foreground">Nenhum</p>
				</div>
				<div class="rounded-md border border-border p-2 text-center">
					<p class="text-sm font-semibold">{info.embedding_status_counts.current}</p>
					<p class="text-xs text-muted-foreground">Atual</p>
				</div>
				<div class="rounded-md border border-border p-2 text-center">
					<p class="text-sm font-semibold">{info.embedding_status_counts.stale}</p>
					<p class="text-xs text-muted-foreground">Desatualizado</p>
				</div>
			</div>

			<div class="mt-4 flex flex-wrap items-center gap-3">
				<Button variant="outline" size="sm" onclick={reindex} disabled={reindexing}>
					{reindexing ? 'Reindexando…' : 'Reindexar FTS5'}
				</Button>
				{#if reindexMessage}
					<p class="text-sm text-muted-foreground">{reindexMessage}</p>
				{/if}
			</div>
		</section>

		<section class="mt-4 rounded-lg border border-border bg-card p-4">
			<h2 class="text-sm font-medium text-muted-foreground">Backup e restore</h2>
			<p class="mt-1 text-xs text-muted-foreground">
				O backup baixa uma cópia consistente do banco SQLite (metadados, tags, anotações e
				configurações — os arquivos PDF em si não estão nele). Restaurar substitui todos os
				dados atuais e reinicia o servidor.
			</p>

			<div class="mt-3 flex flex-wrap items-center gap-3">
				<a href="/api/admin/backup" download class={buttonVariants({ variant: 'outline', size: 'sm' })}>
					<i class="bx bx-download mr-1"></i> Baixar backup
				</a>

				<input
					bind:this={restoreInput}
					type="file"
					accept=".db,.sqlite,.sqlite3,application/vnd.sqlite3,application/octet-stream"
					class="hidden"
					id="admin-restore-input"
					onchange={handleRestoreFile}
				/>
				<label
					for="admin-restore-input"
					class={buttonVariants({ variant: 'destructive', size: 'sm' }) + (restoring ? ' pointer-events-none opacity-50' : ' cursor-pointer')}
				>
					<i class="bx bx-upload mr-1"></i>
					{restoring ? 'Restaurando…' : 'Restaurar backup'}
				</label>
			</div>
			{#if restoreMessage}
				<p class="mt-2 text-sm text-muted-foreground">{restoreMessage}</p>
			{/if}
			{#if restoreError}
				<p class="mt-2 text-sm text-destructive">{restoreError}</p>
			{/if}
		</section>

		<section class="mt-4 rounded-lg border border-border bg-card p-4">
			<h2 class="text-sm font-medium text-muted-foreground">Compartilhamentos</h2>

			{#if loadError}
				<p class="mt-2 text-sm text-destructive">{loadError}</p>
			{/if}

			{#if shares.length === 0}
				<p class="mt-4 text-center text-sm text-muted-foreground">Nenhum PDF compartilhado.</p>
			{:else}
				<ul class="mt-3 divide-y divide-border">
					{#each shares as share (share.id)}
						<li class="flex flex-col gap-2 py-3 sm:flex-row sm:items-center sm:justify-between">
							<div class="min-w-0">
								<p class="truncate text-sm font-medium">{share.pdf_name}</p>
								<p class="text-xs text-muted-foreground">
									{share.views} visualizações · criado em {formatDate(share.created_at)}
								</p>
								<a
									href={shareUrl(share.id)}
									target="_blank"
									rel="noreferrer"
									class="block truncate text-xs text-muted-foreground underline"
								>
									{shareUrl(share.id)}
								</a>
							</div>
							<div class="flex shrink-0 items-center gap-2">
								<Button variant="ghost" size="sm" onclick={() => copyLink(share.id)}>
									<i class="bx bx-copy mr-1"></i>
									{copiedId === share.id ? 'Copiado!' : 'Copiar'}
								</Button>
								<Button
									variant="destructive"
									size="sm"
									onclick={() => revoke(share)}
									disabled={revoking === share.id}
								>
									{revoking === share.id ? 'Revogando…' : 'Revogar'}
								</Button>
							</div>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{/if}
</div>
