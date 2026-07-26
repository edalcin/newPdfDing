<script lang="ts">
	// Combobox de tags: digita para filtrar as tags já existentes, clica para
	// selecionar, ou cria uma tag nova com o texto digitado (o backend já
	// cria qualquer tag inexistente ao salvar — ver internal/store/tags.go,
	// ensureTags — então "criar" aqui é só UX, sem chamada extra à API).
	import { onMount } from 'svelte';
	import { apiJSON } from '$lib/api';
	import type { TagWithCount } from '$lib/types';

	let { value, onChange, id }: { value: string; onChange: (names: string[]) => void; id?: string } = $props();

	let allTags = $state<TagWithCount[]>([]);
	let inputEl = $state<HTMLInputElement>();
	let inputText = $state('');
	let open = $state(false);

	const selected = $derived(value.split(/\s+/).filter(Boolean));

	const suggestions = $derived.by(() => {
		const q = inputText.trim().toLowerCase();
		const selectedLower = new Set(selected.map((s) => s.toLowerCase()));
		return allTags
			.map((t) => t.name)
			.filter((name) => !selectedLower.has(name.toLowerCase()))
			.filter((name) => q === '' || name.toLowerCase().includes(q))
			.slice(0, 8);
	});

	const exactMatch = $derived(
		inputText.trim() !== '' && allTags.some((t) => t.name.toLowerCase() === inputText.trim().toLowerCase())
	);

	onMount(async () => {
		try {
			allTags = await apiJSON<TagWithCount[]>('/tags');
		} catch {
			// sem sugestões — o campo continua utilizável como texto livre
		}
	});

	function addTag(name: string) {
		const trimmed = name.trim();
		if (!trimmed) return;
		if (selected.some((s) => s.toLowerCase() === trimmed.toLowerCase())) {
			inputText = '';
			return;
		}
		onChange([...selected, trimmed]);
		inputText = '';
		open = false;
		inputEl?.focus();
	}

	function removeTag(name: string) {
		onChange(selected.filter((s) => s !== name));
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' || e.key === ',') {
			e.preventDefault();
			if (inputText.trim()) addTag(inputText);
			return;
		}
		if (e.key === 'Backspace' && inputText === '' && selected.length > 0) {
			removeTag(selected[selected.length - 1]);
			return;
		}
		if (e.key === 'Escape') {
			open = false;
		}
	}
</script>

<div class="relative">
	<div
		class="flex min-h-9 flex-wrap items-center gap-1 rounded-md border border-input bg-background px-2 py-1 shadow-sm focus-within:ring-1 focus-within:ring-ring"
	>
		{#each selected as name (name)}
			<span class="flex items-center gap-1 rounded bg-secondary px-1.5 py-0.5 text-xs text-secondary-foreground">
				{name}
				<button
					type="button"
					class="text-muted-foreground hover:text-foreground"
					aria-label={`Remover tag ${name}`}
					onclick={() => removeTag(name)}
				>
					<i class="bx bx-x"></i>
				</button>
			</span>
		{/each}
		<input
			bind:this={inputEl}
			{id}
			bind:value={inputText}
			onfocus={() => (open = true)}
			onblur={() => setTimeout(() => (open = false), 150)}
			onkeydown={handleKeydown}
			placeholder={selected.length === 0 ? 'tag1 tag2…' : ''}
			class="min-w-20 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
		/>
	</div>

	{#if open && (suggestions.length > 0 || (inputText.trim() && !exactMatch))}
		<ul
			class="absolute z-10 mt-1 max-h-56 w-full overflow-auto rounded-md border border-border bg-card p-1 text-sm shadow-md"
		>
			{#each suggestions as name (name)}
				<li>
					<button
						type="button"
						class="w-full rounded px-2 py-1 text-left hover:bg-accent hover:text-accent-foreground"
						onmousedown={(e) => e.preventDefault()}
						onclick={() => addTag(name)}
					>
						{name}
					</button>
				</li>
			{/each}
			{#if inputText.trim() && !exactMatch}
				<li>
					<button
						type="button"
						class="w-full rounded px-2 py-1 text-left text-muted-foreground hover:bg-accent hover:text-accent-foreground"
						onmousedown={(e) => e.preventDefault()}
						onclick={() => addTag(inputText)}
					>
						<i class="bx bx-plus"></i> Criar tag "{inputText.trim()}"
					</button>
				</li>
			{/if}
		</ul>
	{/if}
</div>
