<script lang="ts">
	import { onMount } from 'svelte';
	import { apiJSON, ApiError } from '$lib/api';
	import { theme, type ThemeSetting } from '$lib/theme.svelte';
	import { Button } from '$lib/components/ui/button';
	import type { AIModel, AIModels, Layout, PDFSort, Settings } from '$lib/types';

	const SORT_OPTIONS: { value: PDFSort; label: string }[] = [
		{ value: 'newest', label: 'Mais recentes' },
		{ value: 'oldest', label: 'Mais antigos' },
		{ value: 'name_asc', label: 'Nome (A-Z)' },
		{ value: 'name_desc', label: 'Nome (Z-A)' },
		{ value: 'most_viewed', label: 'Mais vistos' },
		{ value: 'least_viewed', label: 'Menos vistos' },
		{ value: 'recently_viewed', label: 'Vistos recentemente' }
	];

	let layout = $state<Layout>('grid');
	let pdfSorting = $state<PDFSort>('newest');
	let annotationSorting = $state<PDFSort>('newest');
	let inverted = $state(false);
	let keepAwake = $state(false);
	let showProgressBars = $state(true);
	let textModel = $state('');
	let aiModels = $state<AIModels>({ text: [] });
	let aiError = $state('');

	let loaded = $state(false);
	let status = $state('');
	let statusIsError = $state(false);

	onMount(async () => {
		try {
			const settings = await apiJSON<Settings>('/settings');
			layout = settings['ui.layout'];
			pdfSorting = settings['pdf.sorting'];
			annotationSorting = settings['annotation.sorting'] as PDFSort;
			inverted = settings['viewer.inverted'] === '1';
			keepAwake = settings['viewer.keep_awake'] === '1';
			showProgressBars = settings['ui.show_progress_bars'] === '1';
			textModel = settings['ai.text_model'];
		} catch {
			// defaults stand
		} finally {
			loaded = true;
		}

		try {
			aiModels = await apiJSON<AIModels>('/ai/models');
		} catch (err) {
			aiError =
				err instanceof ApiError && err.status === 412
					? 'Defina GEMINI_API_KEY no servidor para habilitar a seleção de modelos.'
					: 'Não foi possível listar os modelos da API Gemini.';
		}
	});

	async function patch(key: string, value: string) {
		try {
			await apiJSON('/settings', { method: 'PATCH', body: { [key]: value } });
			status = 'Salvo.';
			statusIsError = false;
		} catch (err) {
			status = err instanceof ApiError ? err.message : 'Falha ao salvar.';
			statusIsError = true;
		}
	}

	function setTheme(next: ThemeSetting) {
		theme.set(next);
	}

	function setLayout(next: Layout) {
		layout = next;
		patch('ui.layout', next);
	}

	function setPdfSorting(next: PDFSort) {
		pdfSorting = next;
		patch('pdf.sorting', next);
	}

	function setAnnotationSorting(next: PDFSort) {
		annotationSorting = next;
		patch('annotation.sorting', next);
	}

	function setInverted(next: boolean) {
		inverted = next;
		patch('viewer.inverted', next ? '1' : '0');
	}

	function setKeepAwake(next: boolean) {
		keepAwake = next;
		patch('viewer.keep_awake', next ? '1' : '0');
	}

	function setShowProgressBars(next: boolean) {
		showProgressBars = next;
		patch('ui.show_progress_bars', next ? '1' : '0');
	}

	function setTextModel(next: string) {
		textModel = next;
		patch('ai.text_model', next);
	}

	// Garante que o valor salvo apareça mesmo que models.list não o devolva
	// (chave trocada, modelo retirado do catálogo).
	function withCurrent(list: AIModel[], current: string): AIModel[] {
		if (!current || list.some((m) => m.name === current)) return list;
		return [{ name: current, display_name: `${current} (indisponível)` }, ...list];
	}
	const textOptions = $derived(withCurrent(aiModels.text, textModel));

</script>

<div class="mx-auto max-w-2xl p-4">
	<h1 class="text-lg font-semibold">Preferências</h1>
	{#if status}
		<p class={`mt-1 text-sm ${statusIsError ? 'text-destructive' : 'text-muted-foreground'}`}>
			{status}
		</p>
	{/if}

	{#if loaded}
		<section class="mt-6">
			<h2 class="text-sm font-semibold text-muted-foreground">Aparência</h2>
			<div class="mt-3 space-y-4 rounded-lg border border-border p-4">
				<div class="flex items-center justify-between gap-4">
					<span class="text-sm">Tema</span>
					<select
						class="h-9 rounded-md border border-input bg-background px-3 text-sm"
						value={theme.value}
						onchange={(e) => setTheme((e.target as HTMLSelectElement).value as ThemeSetting)}
					>
						<option value="system">Sistema</option>
						<option value="light">Claro</option>
						<option value="dark">Escuro</option>
					</select>
				</div>
				<div class="flex items-center justify-between gap-4">
					<span class="text-sm">Layout padrão da biblioteca</span>
					<select
						class="h-9 rounded-md border border-input bg-background px-3 text-sm"
						value={layout}
						onchange={(e) => setLayout((e.target as HTMLSelectElement).value as Layout)}
					>
						<option value="grid">Grade</option>
						<option value="list">Lista</option>
						<option value="compact">Compacto</option>
						<option value="minimal">Mínimo</option>
					</select>
				</div>
			</div>
		</section>

		<section class="mt-6">
			<h2 class="text-sm font-semibold text-muted-foreground">Biblioteca</h2>
			<div class="mt-3 space-y-4 rounded-lg border border-border p-4">
				<div class="flex items-center justify-between gap-4">
					<span class="text-sm">Ordenação padrão de PDFs</span>
					<select
						class="h-9 rounded-md border border-input bg-background px-3 text-sm"
						value={pdfSorting}
						onchange={(e) => setPdfSorting((e.target as HTMLSelectElement).value as PDFSort)}
					>
						{#each SORT_OPTIONS as opt (opt.value)}
							<option value={opt.value}>{opt.label}</option>
						{/each}
					</select>
				</div>
				<div class="flex items-center justify-between gap-4">
					<span class="text-sm">Ordenação padrão de anotações</span>
					<select
						class="h-9 rounded-md border border-input bg-background px-3 text-sm"
						value={annotationSorting}
						onchange={(e) =>
							setAnnotationSorting((e.target as HTMLSelectElement).value as PDFSort)}
					>
						{#each SORT_OPTIONS as opt (opt.value)}
							<option value={opt.value}>{opt.label}</option>
						{/each}
					</select>
				</div>
				<label class="flex items-center justify-between gap-4 text-sm">
					<span>Barras de progresso nos cards</span>
					<input
						type="checkbox"
						checked={showProgressBars}
						onchange={(e) => setShowProgressBars((e.target as HTMLInputElement).checked)}
					/>
				</label>
			</div>
		</section>

		<section class="mt-6">
			<h2 class="text-sm font-semibold text-muted-foreground">Visualizador</h2>
			<div class="mt-3 space-y-4 rounded-lg border border-border p-4">
				<label class="flex items-center justify-between gap-4 text-sm">
					<span>Modo invertido</span>
					<input
						type="checkbox"
						checked={inverted}
						onchange={(e) => setInverted((e.target as HTMLInputElement).checked)}
					/>
				</label>
				<label class="flex items-center justify-between gap-4 text-sm">
					<span>Manter tela ligada</span>
					<input
						type="checkbox"
						checked={keepAwake}
						onchange={(e) => setKeepAwake((e.target as HTMLInputElement).checked)}
					/>
				</label>
			</div>
		</section>

		<section class="mt-6">
			<h2 class="text-sm font-semibold text-muted-foreground">IA</h2>
			<div class="mt-3 space-y-4 rounded-lg border border-border p-4">
				{#if aiError}
					<p class="text-sm text-muted-foreground">{aiError}</p>
				{/if}
				<div class="flex items-center justify-between gap-4">
					<span class="text-sm">Modelo para descrição e sugestão de tags</span>
					<select
						class="h-9 rounded-md border border-input bg-background px-3 text-sm"
						value={textModel}
						disabled={!!aiError}
						onchange={(e) => setTextModel((e.target as HTMLSelectElement).value)}
					>
						<option value="">— não selecionado —</option>
						{#each textOptions as m (m.name)}
							<option value={m.name}>{m.display_name}</option>
						{/each}
					</select>
				</div>
			</div>
		</section>
	{/if}

</div>
