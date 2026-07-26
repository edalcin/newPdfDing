<script lang="ts">
	import { onMount } from 'svelte';
	import { apiJSON, ApiError } from '$lib/api';
	import type { TagWithCount } from '$lib/types';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';

	interface TreeNode {
		segment: string;
		path: string;
		tag: TagWithCount | null;
		children: Map<string, TreeNode>;
	}

	let tags = $state<TagWithCount[]>([]);
	let loading = $state(true);
	let treeMode = $state(false);
	let rowErrors = $state<Record<string, string>>({});

	let mergeFrom = $state('');
	let mergeTo = $state('');
	let mergeError = $state('');
	let merging = $state(false);

	const sortedTags = $derived([...tags].sort((a, b) => a.name.localeCompare(b.name)));

	onMount(async () => {
		try {
			const [tagList, settings] = await Promise.all([
				apiJSON<TagWithCount[]>('/tags'),
				apiJSON<Record<string, string>>('/settings')
			]);
			tags = tagList;
			treeMode = settings['ui.tag_tree_mode'] === '1';
		} finally {
			loading = false;
		}
	});

	function messageFor(err: unknown, conflictMessage: string, fallback: string): string {
		if (err instanceof ApiError && err.status === 409) return conflictMessage;
		return err instanceof Error ? err.message : fallback;
	}

	async function toggleTreeMode() {
		const next = treeMode ? '0' : '1';
		treeMode = !treeMode;
		try {
			await apiJSON('/settings', { method: 'PATCH', body: { 'ui.tag_tree_mode': next } });
		} catch {
			// best-effort persistence; local toggle stands regardless
		}
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

	// Nomes de tag podem conter '/' para hierarquia; a árvore é puramente
	// derivada no cliente (refatoracao/05-api.md, "Tags" — notas). Todo nó
	// existe fisicamente como tag apenas se `node.tag` estiver preenchido —
	// segmentos intermediários sem tag própria são só agrupamento visual.
	function buildTree(list: TagWithCount[]): TreeNode {
		const root: TreeNode = { segment: '', path: '', tag: null, children: new Map() };
		for (const tag of list) {
			let node = root;
			let path = '';
			for (const part of tag.name.split('/').filter(Boolean)) {
				path = path ? `${path}/${part}` : part;
				let child = node.children.get(part);
				if (!child) {
					child = { segment: part, path, tag: null, children: new Map() };
					node.children.set(part, child);
				}
				node = child;
			}
			node.tag = tag;
		}
		return root;
	}

	function sortedChildren(node: TreeNode): TreeNode[] {
		return [...node.children.values()].sort((a, b) => a.segment.localeCompare(b.segment));
	}

	const tree = $derived(buildTree(tags));
</script>

{#snippet tagRow(tag: TagWithCount)}
	<div class="flex items-center gap-2">
		<Input
			value={tag.name}
			onblur={(e) => renameTag(tag, (e.target as HTMLInputElement).value)}
			class="flex-1"
			aria-label="Nome da tag"
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

{#snippet treeNode(node: TreeNode)}
	<li>
		{#if node.tag}
			{@render tagRow(node.tag)}
		{/if}
		{#if node.children.size > 0}
			{#if node.tag}
				<details class="ml-1 mt-1">
					<summary class="cursor-pointer text-xs text-muted-foreground">{node.children.size} subtag(s)</summary>
					<ul class="mt-1 flex flex-col gap-2 border-l border-border pl-4">
						{#each sortedChildren(node) as child (child.path)}
							{@render treeNode(child)}
						{/each}
					</ul>
				</details>
			{:else}
				<details open>
					<summary class="cursor-pointer py-1 text-sm font-medium">{node.segment}</summary>
					<ul class="mt-1 flex flex-col gap-2 border-l border-border pl-4">
						{#each sortedChildren(node) as child (child.path)}
							{@render treeNode(child)}
						{/each}
					</ul>
				</details>
			{/if}
		{/if}
	</li>
{/snippet}

<div class="mx-auto max-w-3xl p-4">
	<div class="flex items-center justify-between">
		<h1 class="text-lg font-semibold">Tags</h1>
		<Button variant="outline" size="sm" onclick={toggleTreeMode}>
			<i class={`bx ${treeMode ? 'bx-list-ul' : 'bx-sitemap'} mr-1`}></i>
			{treeMode ? 'Modo lista' : 'Modo árvore'}
		</Button>
	</div>

	{#if loading}
		<p class="mt-4 text-sm text-muted-foreground">Carregando…</p>
	{:else if tags.length === 0}
		<p class="mt-4 text-sm text-muted-foreground">Nenhuma tag ainda.</p>
	{:else if treeMode}
		<ul class="mt-4 flex flex-col gap-2">
			{#each sortedChildren(tree) as child (child.path)}
				{@render treeNode(child)}
			{/each}
		</ul>
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
		<div class="mt-2 flex flex-wrap items-center gap-2">
			<select bind:value={mergeFrom} class="h-9 rounded-md border border-input bg-background px-3 text-sm">
				<option value="">De…</option>
				{#each sortedTags as t (t.id)}
					<option value={t.id}>{t.name}</option>
				{/each}
			</select>
			<i class="bx bx-right-arrow-alt"></i>
			<select bind:value={mergeTo} class="h-9 rounded-md border border-input bg-background px-3 text-sm">
				<option value="">Para…</option>
				{#each sortedTags as t (t.id)}
					<option value={t.id}>{t.name}</option>
				{/each}
			</select>
			<Button onclick={mergeTags} disabled={merging || !mergeFrom || !mergeTo || mergeFrom === mergeTo}>Fundir</Button>
		</div>
		{#if mergeError}
			<p class="mt-1 text-sm text-destructive">{mergeError}</p>
		{/if}
	</div>
</div>
