<script lang="ts">
	import { onMount } from 'svelte';
	import { apiJSON, ApiError } from '$lib/api';
	import type { TagWithCount } from '$lib/types';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';

	let tags = $state<TagWithCount[]>([]);
	let loading = $state(true);
	let rowErrors = $state<Record<string, string>>({});

	let mergeFrom = $state('');
	let mergeTo = $state('');
	let mergeError = $state('');
	let merging = $state(false);

	const sortedTags = $derived([...tags].sort((a, b) => a.name.localeCompare(b.name)));

	onMount(async () => {
		try {
			tags = await apiJSON<TagWithCount[]>('/tags');
		} finally {
			loading = false;
		}
	});

	function messageFor(err: unknown, conflictMessage: string, fallback: string): string {
		if (err instanceof ApiError && err.status === 409) return conflictMessage;
		return err instanceof Error ? err.message : fallback;
	}

	async function renameTag(tag: TagWithCount, name: string) {
		rowErrors = { ...rowErrors, [tag.id]: '' };
		if (!name.trim() || name === tag.name) return;
		try {
			const updated = await apiJSON<TagWithCount>(`/tags/${tag.id}`, { method: 'PATCH', body: { name } });
			tags = tags.map((t) => (t.id === tag.id ? { ...t, ...updated } : t));
		} catch (err) {
			rowErrors = { ...rowErrors, [tag.id]: messageFor(err, 'nome já em uso', 'Falha ao renomear') };
		}
	}

	async function deleteTag(tag: TagWithCount) {
		if (!confirm(`Excluir a tag "${tag.name}"?`)) return;
		rowErrors = { ...rowErrors, [tag.id]: '' };
		try {
			await apiJSON(`/tags/${tag.id}`, { method: 'DELETE' });
			tags = tags.filter((t) => t.id !== tag.id);
		} catch (err) {
			rowErrors = { ...rowErrors, [tag.id]: err instanceof Error ? err.message : 'Falha ao excluir' };
		}
	}

	async function refetchTags() {
		tags = await apiJSON<TagWithCount[]>('/tags');
	}

	async function mergeTags() {
		mergeError = '';
		if (!mergeFrom || !mergeTo || mergeFrom === mergeTo) return;
		const fromTag = tags.find((t) => t.id === mergeFrom);
		const toTag = tags.find((t) => t.id === mergeTo);
		if (!confirm(`Fundir "${fromTag?.name}" em "${toTag?.name}"? Essa ação não pode ser desfeita.`)) return;
		merging = true;
		try {
			await apiJSON('/tags/substitute', { method: 'POST', body: { from_id: mergeFrom, to_id: mergeTo } });
			mergeFrom = '';
			mergeTo = '';
			await refetchTags();
		} catch (err) {
			mergeError = err instanceof Error ? err.message : 'Falha ao fundir tags';
		} finally {
			merging = false;
		}
	}
</script>

{#snippet tagRow(tag: TagWithCount)}
	<div class="flex items-center gap-2">
		<Input
			value={tag.name}
			onblur={(e) => renameTag(tag, (e.target as HTMLInputElement).value)}
			class="flex-1"
		/>
		<span class="w-10 shrink-0 text-right text-xs text-muted-foreground">{tag.count}</span>
		<Button variant="ghost" size="icon" aria-label="Excluir tag" onclick={() => deleteTag(tag)}>
			<i class="bx bx-trash"></i>
		</Button>
	</div>
	{#if rowErrors[tag.id]}
		<p class="mt-1 text-sm text-destructive">{rowErrors[tag.id]}</p>
	{/if}
{/snippet}

<div class="mx-auto max-w-3xl p-4">
	<h1 class="text-lg font-semibold">Tags</h1>

	{#if loading}
		<p class="mt-4 text-sm text-muted-foreground">Carregando…</p>
	{:else if tags.length === 0}
		<p class="mt-4 text-sm text-muted-foreground">Nenhuma tag ainda.</p>
	{:else}
		<ul class="mt-4 flex flex-col gap-2">
			{#each sortedTags as tag (tag.id)}
				<li class="rounded-lg border border-border p-3">
					{@render tagRow(tag)}
				</li>
			{/each}
		</ul>
	{/if}

	<div class="mt-6 rounded-lg border border-border p-3">
		<h2 class="text-sm font-semibold">Fundir tags</h2>
		<p class="mt-1 text-xs text-muted-foreground">
			Move todos os PDFs de uma tag para outra e remove a tag de origem.
		</p>
		<div class="mt-3 flex flex-wrap items-center gap-2">
			<select
				class="h-9 rounded-md border border-input bg-background px-3 text-sm"
				bind:value={mergeFrom}
			>
				<option value="">De…</option>
				{#each sortedTags as tag (tag.id)}
					<option value={tag.id}>{tag.name}</option>
				{/each}
			</select>
			<i class="bx bx-right-arrow-alt"></i>
			<select
				class="h-9 rounded-md border border-input bg-background px-3 text-sm"
				bind:value={mergeTo}
			>
				<option value="">Para…</option>
				{#each sortedTags as tag (tag.id)}
					<option value={tag.id}>{tag.name}</option>
				{/each}
			</select>
			<Button
				size="sm"
				disabled={!mergeFrom || !mergeTo || mergeFrom === mergeTo || merging}
				onclick={mergeTags}
			>
				{merging ? 'Fundindo…' : 'Fundir'}
			</Button>
		</div>
		{#if mergeError}<p class="mt-2 text-sm text-destructive">{mergeError}</p>{/if}
	</div>
</div>
