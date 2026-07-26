<script lang="ts">
	import { onMount } from 'svelte';
	import { apiJSON, ApiError } from '$lib/api';
	import type { Collection } from '$lib/types';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';

	let collections = $state<Collection[]>([]);
	let loading = $state(true);
	let rowErrors = $state<Record<string, string>>({});

	let newName = $state('');
	let newDescription = $state('');
	let createError = $state('');
	let creating = $state(false);

	onMount(async () => {
		try {
			collections = await apiJSON<Collection[]>('/collections');
		} finally {
			loading = false;
		}
	});

	function messageFor(err: unknown, conflictMessage: string, fallback: string): string {
		if (err instanceof ApiError && err.status === 409) return conflictMessage;
		return err instanceof Error ? err.message : fallback;
	}

	async function createCollection() {
		const name = newName.trim();
		if (!name) return;
		createError = '';
		creating = true;
		try {
			const created = await apiJSON<Collection>('/collections', {
				method: 'POST',
				body: { name, description: newDescription.trim() }
			});
			collections = [created, ...collections];
			newName = '';
			newDescription = '';
		} catch (err) {
			createError = messageFor(err, 'nome já em uso', 'Falha ao criar coleção');
		} finally {
			creating = false;
		}
	}

	async function updateField(c: Collection, field: 'name' | 'description', value: string) {
		rowErrors = { ...rowErrors, [c.id]: '' };
		if (value === c[field]) return;
		try {
			const updated = await apiJSON<Collection>(`/collections/${c.id}`, {
				method: 'PATCH',
				body: { [field]: value }
			});
			// PATCH does not recompute pdf_count (unrelated to the field being
			// edited) — keep the count already shown instead of overwriting it
			// with the response's zero value.
			collections = collections.map((x) => (x.id === c.id ? { ...updated, pdf_count: x.pdf_count } : x));
		} catch (err) {
			rowErrors = { ...rowErrors, [c.id]: messageFor(err, 'nome já em uso', 'Falha ao salvar') };
		}
	}

	async function deleteCollection(c: Collection) {
		if (!confirm(`Excluir a coleção "${c.name}"?`)) return;
		rowErrors = { ...rowErrors, [c.id]: '' };
		try {
			await apiJSON(`/collections/${c.id}`, { method: 'DELETE' });
			collections = collections.filter((x) => x.id !== c.id);
		} catch (err) {
			rowErrors = {
				...rowErrors,
				[c.id]: messageFor(err, 'não é possível excluir a coleção padrão', 'Falha ao excluir')
			};
		}
	}
</script>

<div class="mx-auto max-w-3xl p-4">
	<h1 class="text-lg font-semibold">Coleções</h1>

	<form
		class="mt-4 flex flex-wrap items-end gap-2 rounded-lg border border-border p-3"
		onsubmit={(e) => {
			e.preventDefault();
			createCollection();
		}}
	>
		<div class="min-w-[10rem] flex-1">
			<label for="new-collection-name" class="text-xs text-muted-foreground">Nome</label>
			<Input id="new-collection-name" bind:value={newName} placeholder="Nome da coleção" />
		</div>
		<div class="min-w-[10rem] flex-1">
			<label for="new-collection-description" class="text-xs text-muted-foreground">Descrição</label>
			<Input id="new-collection-description" bind:value={newDescription} placeholder="Descrição (opcional)" />
		</div>
		<Button type="submit" disabled={creating || !newName.trim()}>Criar</Button>
	</form>
	{#if createError}
		<p class="mt-1 text-sm text-destructive">{createError}</p>
	{/if}

	{#if loading}
		<p class="mt-4 text-sm text-muted-foreground">Carregando…</p>
	{:else if collections.length === 0}
		<p class="mt-4 text-sm text-muted-foreground">Nenhuma coleção ainda.</p>
	{:else}
		<ul class="mt-4 flex flex-col gap-2">
			{#each collections as c (c.id)}
				<li class="rounded-lg border border-border p-3">
					<div class="flex items-start justify-between gap-2">
						<div class="flex-1">
							<div class="flex items-center gap-2">
								<Input
									value={c.name}
									onblur={(e) => updateField(c, 'name', (e.target as HTMLInputElement).value)}
									class="font-medium"
									aria-label="Nome da coleção"
								/>
							{#if c.is_default}
								<span class="shrink-0 rounded bg-secondary px-2 py-0.5 text-xs text-secondary-foreground">padrão</span>
							{/if}
							<span class="shrink-0 text-xs text-muted-foreground">
								{c.pdf_count} {c.pdf_count === 1 ? 'PDF' : 'PDFs'}
							</span>
							</div>
							<Input
								value={c.description}
								onblur={(e) => updateField(c, 'description', (e.target as HTMLInputElement).value)}
								placeholder="Descrição"
								class="mt-1 text-sm text-muted-foreground"
								aria-label="Descrição da coleção"
							/>
						</div>
						<Button variant="ghost" size="icon" aria-label="Excluir coleção" onclick={() => deleteCollection(c)}>
							<i class="bx bx-trash"></i>
						</Button>
					</div>
					{#if rowErrors[c.id]}
						<p class="mt-1 text-sm text-destructive">{rowErrors[c.id]}</p>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</div>
