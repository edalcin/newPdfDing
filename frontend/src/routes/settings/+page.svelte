<script lang="ts">
	import { onMount } from 'svelte';
	import { apiJSON, ApiError } from '$lib/api';
	import { theme, type ThemeSetting } from '$lib/theme.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import type { Layout, PDFSort, Settings, Signature } from '$lib/types';

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
	let tagTreeMode = $state(false);

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
			tagTreeMode = settings['ui.tag_tree_mode'] === '1';
		} catch {
			// defaults stand
		} finally {
			loaded = true;
		}
		await loadSignatures();
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

	function setTagTreeMode(next: boolean) {
		tagTreeMode = next;
		patch('ui.tag_tree_mode', next ? '1' : '0');
	}

	// --- Assinaturas ---
	let signatures = $state<Signature[]>([]);
	let signaturesError = $state('');
	let canvasEl = $state<HTMLCanvasElement>();
	let drawing = false;
	let hasStroke = $state(false);
	let sigName = $state('');
	let savingSignature = $state(false);

	async function loadSignatures() {
		try {
			signatures = await apiJSON<Signature[]>('/signatures');
		} catch (err) {
			signaturesError = err instanceof ApiError ? err.message : 'Falha ao carregar assinaturas.';
		}
	}

	function ctx() {
		return canvasEl?.getContext('2d') ?? null;
	}

	function pointerPos(e: PointerEvent) {
		const rect = canvasEl!.getBoundingClientRect();
		return { x: e.clientX - rect.left, y: e.clientY - rect.top };
	}

	function startStroke(e: PointerEvent) {
		const c = ctx();
		if (!c || !canvasEl) return;
		drawing = true;
		canvasEl.setPointerCapture(e.pointerId);
		const { x, y } = pointerPos(e);
		c.beginPath();
		c.moveTo(x, y);
	}

	function moveStroke(e: PointerEvent) {
		if (!drawing) return;
		const c = ctx();
		if (!c) return;
		const { x, y } = pointerPos(e);
		c.lineWidth = 2;
		c.lineCap = 'round';
		c.strokeStyle = '#000000';
		c.lineTo(x, y);
		c.stroke();
		hasStroke = true;
	}

	function endStroke(e: PointerEvent) {
		drawing = false;
		canvasEl?.releasePointerCapture(e.pointerId);
	}

	function clearCanvas() {
		const c = ctx();
		if (!c || !canvasEl) return;
		c.clearRect(0, 0, canvasEl.width, canvasEl.height);
		hasStroke = false;
	}

	async function saveSignature() {
		if (!canvasEl || !hasStroke || !sigName.trim()) return;
		savingSignature = true;
		signaturesError = '';
		try {
			const dataUrl = canvasEl.toDataURL('image/png');
			const created = await apiJSON<Signature>('/signatures', {
				method: 'POST',
				body: { name: sigName.trim(), data: dataUrl }
			});
			signatures = [created, ...signatures];
			sigName = '';
			clearCanvas();
		} catch (err) {
			signaturesError = err instanceof ApiError ? err.message : 'Falha ao salvar assinatura.';
		} finally {
			savingSignature = false;
		}
	}

	async function deleteSignature(sig: Signature) {
		if (!confirm(`Excluir a assinatura "${sig.name}"?`)) return;
		try {
			await apiJSON(`/signatures/${sig.id}`, { method: 'DELETE' });
			signatures = signatures.filter((s) => s.id !== sig.id);
		} catch (err) {
			signaturesError = err instanceof ApiError ? err.message : 'Falha ao excluir assinatura.';
		}
	}
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
				<label class="flex items-center justify-between gap-4 text-sm">
					<span>Modo árvore de tags</span>
					<input
						type="checkbox"
						checked={tagTreeMode}
						onchange={(e) => setTagTreeMode((e.target as HTMLInputElement).checked)}
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
	{/if}

	<section class="mt-6">
		<h2 class="text-sm font-semibold text-muted-foreground">Assinaturas</h2>
		<div class="mt-3 rounded-lg border border-border p-4">
			{#if signaturesError}
				<p class="mb-3 text-sm text-destructive">{signaturesError}</p>
			{/if}

			{#if signatures.length > 0}
				<ul class="mb-4 space-y-2">
					{#each signatures as sig (sig.id)}
						<li class="flex items-center justify-between gap-3 rounded-md border border-border p-2">
							<div class="flex items-center gap-3">
								<img src={sig.data} alt={sig.name} class="h-12 rounded bg-white" />
								<span class="text-sm">{sig.name}</span>
							</div>
							<Button
								variant="ghost"
								size="sm"
								class="text-destructive hover:text-destructive"
								onclick={() => deleteSignature(sig)}
							>
								<i class="bx bx-trash"></i>
								Excluir
							</Button>
						</li>
					{/each}
				</ul>
			{:else}
				<p class="mb-4 text-sm text-muted-foreground">Nenhuma assinatura salva ainda.</p>
			{/if}

			<div class="space-y-3">
				<canvas
					bind:this={canvasEl}
					width="400"
					height="150"
					class="touch-none rounded-md border border-border bg-white"
					onpointerdown={startStroke}
					onpointermove={moveStroke}
					onpointerup={endStroke}
					onpointerleave={endStroke}
				></canvas>
				<div class="flex flex-wrap items-center gap-2">
					<Button type="button" variant="outline" size="sm" onclick={clearCanvas}>
						Limpar
					</Button>
					<Input
						type="text"
						placeholder="Nome da assinatura"
						bind:value={sigName}
						class="flex-1"
					/>
					<Button
						type="button"
						size="sm"
						disabled={!hasStroke || !sigName.trim() || savingSignature}
						onclick={saveSignature}
					>
						{savingSignature ? 'Salvando…' : 'Salvar assinatura'}
					</Button>
				</div>
			</div>
		</div>
	</section>
</div>
