<script lang="ts">
	// Host do viewer EmbedPDF nativo (sem iframe/postMessage). Este
	// componente possui: busca de metadados do PDF e das anotações
	// existentes, o <PdfViewer> montado como componente Svelte, os toggles
	// de modo invertido e "manter tela ligada", e a persistência
	// bidirecional de anotações via AnnotationCapability.onAnnotationEvent.
	import { page } from '$app/state';
	import { apiJSON, apiRequest } from '$lib/api';
	import { extractAssetsFromUrl } from '$lib/pdf-process';
	import { createAnnotation, patchAnnotation, deleteAnnotation } from '$lib/annotations.svelte';
	import { legacyToTransferItem, toAnnotationPayload } from '$lib/embedpdf';
	import PdfViewer from '$lib/components/pdf-viewer.svelte';
	import { Button } from '$lib/components/ui/button';
	import type {
		AnnotationEvent,
		AnnotationPlugin,
		AnnotationTransferItem,
		DocumentManagerCapability,
		DocumentManagerPlugin,
		PluginRegistry,
		SelectionPlugin
	} from '@embedpdf/svelte-pdf-viewer';
	import type { Annotation, PDF } from '$lib/types';

	// page.params tipa como `string | undefined` porque LayoutParams é
	// compartilhado entre todas as rotas de '/' — nesta rota o segmento
	// [id] está sempre presente em tempo de execução.
	const id = $derived(page.params.id ?? '');

	let pdf = $state<PDF | null>(null);
	let inverted = $state(false);
	let keepAwake = $state(false);
	let errorMsg = $state('');
	let wakeLock: WakeLockSentinel | null = null;

	const wakeLockSupported = typeof navigator !== 'undefined' && 'wakeLock' in navigator;
	const fileUrl = $derived(id ? `/api/pdfs/${id}/file` : '');

	// idMap traduz o id gerado pelo EmbedPDF para o id da linha em
	// pdf_annotations — a importação inicial preserva `annotation.id`
	// (AnnotationTransferItem carrega o objeto inteiro, id incluso).
	let idMap = new Map<string, string>();
	// Durante importAnnotations() na carga inicial, cada anotação reemite um
	// evento 'create' — sem esta guarda o banco duplicaria cada anotação a
	// cada abertura do viewer.
	let suppressPersist = false;
	let lastSelectedText = '';
	const updateTimers = new Map<string, ReturnType<typeof setTimeout>>();
	let pageChangeTimer: ReturnType<typeof setTimeout> | undefined;

	async function requestWakeLock() {
		if (!wakeLockSupported) return;
		try {
			wakeLock = await navigator.wakeLock.request('screen');
		} catch {
			// best-effort — alguns navegadores exigem gesto do usuário ou negam a permissão
		}
	}

	async function releaseWakeLock() {
		if (!wakeLock) return;
		try {
			await wakeLock.release();
		} catch {
			// ignore
		}
		wakeLock = null;
	}

	/** Busca todas as anotações do PDF, paginando até o fim — a lista de um
	 * documento nunca é grande o bastante para justificar rolagem infinita
	 * aqui (ao contrário de /highlights e /comments). */
	async function loadAnnotations(pdfId: string): Promise<Annotation[]> {
		const items: Annotation[] = [];
		let cursor = '';
		for (;;) {
			const params = new URLSearchParams({ pdf_id: pdfId });
			if (cursor) params.set('cursor', cursor);
			const pageResult = await apiJSON<{ items: Annotation[]; next_cursor: string | null }>(
				`/annotations?${params.toString()}`
			);
			items.push(...pageResult.items);
			if (!pageResult.next_cursor) break;
			cursor = pageResult.next_cursor;
		}
		return items;
	}

	async function loadData(pdfId: string) {
		pdf = null;
		errorMsg = '';
		idMap = new Map();
		await releaseWakeLock(); // evita vazar o sentinel se o componente for reaproveitado para outro [id]
		if (!pdfId) return;
		try {
			const [pdfData, settings] = await Promise.all([
				apiJSON<PDF>(`/pdfs/${pdfId}`),
				apiJSON<Record<string, string>>('/settings')
			]);
			pdf = pdfData;
			inverted = settings['viewer.inverted'] === '1';
			keepAwake = settings['viewer.keep_awake'] === '1';
			if (keepAwake) await requestWakeLock();
			// Só reprocessa (2º download integral + extração de texto de todas
			// as páginas) quando o documento ainda não tem texto — PDFs comuns
			// (upload normal) já chegam com has_text=true e preview em alta
			// resolução do processamento no navegador; reprocessar em toda
			// abertura era puro desperdício, concorrendo por CPU/memória com o
			// EmbedPDF na mesma abertura (ver refatoracao/11-desempenho-viewer.md,
			// causa C3). Documentos sem texto (import legado, watch-dir) ainda
			// passam pelo backfill completo — mas só até o texto ser gravado,
			// depois disso has_text vira true e não repete mais.
			if (!pdfData.has_text) backfillAssets(pdfId);
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Falha ao carregar o PDF.';
		}
	}

	/** Extrai texto e um preview em resolução atual no navegador (mesmo
	 * pdf.js do upload) e envia ao servidor — só chamada para documentos que
	 * chegaram sem texto (import do banco legado, watch-dir consumer's pure-Go
	 * extraction gap; ver 05-api.md, "POST .../text"). Melhor-esforço:
	 * silencioso em qualquer falha, nunca bloqueia ou interrompe a abertura
	 * do viewer (ver refatoracao/06-frontend.md, "Degradação graciosa"). */
	async function backfillAssets(pdfId: string) {
		try {
			const { text, preview } = await extractAssetsFromUrl(`/api/pdfs/${pdfId}/file`);
			if (text) {
				await apiJSON(`/pdfs/${pdfId}/text`, { method: 'POST', body: { text } });
				if (pdf && pdf.id === pdfId) pdf = { ...pdf, has_text: true };
			}
			const form = new FormData();
			form.append('preview', preview, 'preview.png');
			await apiRequest(`/pdfs/${pdfId}/preview`, { method: 'POST', body: form });
		} catch {
			// PDF corrompido/protegido, ou alguma requisição falhou — nada
			// atualizado desta vez; a próxima abertura tenta de novo.
		}
	}

	// Recarrega sempre que o segmento [id] mudar (SvelteKit reaproveita a
	// instância do componente entre navegações para a mesma rota).
	$effect(() => {
		loadData(id);
	});

	$effect(() => releaseWakeLock);

	async function toggleInverted() {
		inverted = !inverted;
		try {
			await apiJSON('/settings', { method: 'PATCH', body: { 'viewer.inverted': inverted ? '1' : '0' } });
		} catch {
			// persistência best-effort — mesmo padrão de +page.svelte (layout)
		}
	}

	async function toggleKeepAwake() {
		keepAwake = !keepAwake;
		if (keepAwake) await requestWakeLock();
		else await releaseWakeLock();
		try {
			await apiJSON('/settings', { method: 'PATCH', body: { 'viewer.keep_awake': keepAwake ? '1' : '0' } });
		} catch {
			// persistência best-effort
		}
	}

	function handlePageChange(newPage: number) {
		if (pageChangeTimer) clearTimeout(pageChangeTimer);
		pageChangeTimer = setTimeout(() => {
			apiJSON(`/pdfs/${id}`, { method: 'PATCH', body: { current_page: newPage } }).catch(() => {});
		}, 2000);
	}

	function handleAnnotationEvent(e: AnnotationEvent) {
		if (suppressPersist || e.type === 'loaded') return;

		if (e.type === 'create') {
			// EmbedPDF emite 'create' duas vezes por anotação nova: uma vez
			// otimista (`committed: false`, antes do engine persistir) e de
			// novo após `commit()` confirmar (`committed: true`) — autoCommit
			// está ligado em viewerConfig(). Sem este guard, cada destaque/
			// comentário criado gerava duas linhas em pdf_annotations.
			if (!e.committed) return;
			const item: AnnotationTransferItem = { annotation: e.annotation, ctx: e.ctx };
			const payload = toAnnotationPayload(item, lastSelectedText);
			createAnnotation(id, payload.kind, payload.page, payload.text, {
				note: payload.note,
				color: payload.color,
				data: payload.data
			})
				.then((created) => idMap.set(e.annotation.id, created.id))
				.catch((err) => {
					errorMsg = err instanceof Error ? err.message : 'Falha ao criar anotação.';
				});
			return;
		}

		const rowId = idMap.get(e.annotation.id);
		if (!rowId) return;

		if (e.type === 'update') {
			const existingTimer = updateTimers.get(e.annotation.id);
			if (existingTimer) clearTimeout(existingTimer);
			updateTimers.set(
				e.annotation.id,
				setTimeout(() => {
					updateTimers.delete(e.annotation.id);
					// ponytail: geometria/cor/nota apenas — sem re-anexar `ctx` de
					// carimbo num move/resize (a imagem não muda). Refazer via
					// exportAnnotations() se um gap real aparecer aqui.
					const payload = toAnnotationPayload({ annotation: e.annotation }, '');
					patchAnnotation(rowId, { note: payload.note, color: payload.color, data: payload.data }).catch((err) => {
						errorMsg = err instanceof Error ? err.message : 'Falha ao salvar anotação.';
					});
				}, 500)
			);
			return;
		}

		if (e.type === 'delete') {
			deleteAnnotation(rowId).catch(() => {});
			idMap.delete(e.annotation.id);
		}
	}

	function waitForActiveDocument(docManager: DocumentManagerCapability) {
		const existing = docManager.getActiveDocument();
		if (existing) return Promise.resolve(existing);
		return new Promise<NonNullable<ReturnType<DocumentManagerCapability['getActiveDocument']>>>((resolve) => {
			const off = docManager.onDocumentOpened(() => {
				const doc = docManager.getActiveDocument();
				if (doc) {
					off();
					resolve(doc);
				}
			});
		});
	}

	async function handleViewerReady(registry: PluginRegistry) {
		const annotationCap = registry.getPlugin<AnnotationPlugin>('annotation')!.provides();
		const selectionCap = registry.getPlugin<SelectionPlugin>('selection')!.provides();
		const docManagerCap = registry.getPlugin<DocumentManagerPlugin>('document-manager')!.provides();

		selectionCap.onEndSelection(() => {
			selectionCap
				.getSelectedText()
				.toPromise()
				.then((texts) => {
					lastSelectedText = texts.join(' ');
				})
				.catch(() => {});
		});

		annotationCap.onAnnotationEvent(handleAnnotationEvent);

		try {
			const doc = await waitForActiveDocument(docManagerCap);
			const rows = await loadAnnotations(id);
			const items: AnnotationTransferItem[] = [];
			for (const row of rows) {
				const item = row.data
					? (JSON.parse(row.data) as AnnotationTransferItem)
					: row.page >= 1 && row.page <= doc.pages.length
						? legacyToTransferItem(row, doc.pages[row.page - 1].size)
						: null;
				if (!item) continue;
				idMap.set(item.annotation.id, row.id);
				items.push(item);
			}
			suppressPersist = true;
			annotationCap.importAnnotations(items);
			suppressPersist = false;
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Falha ao carregar as anotações.';
		}
	}
</script>

<div class="flex h-[calc(100vh-3.5rem)] flex-col">
	<div class="flex shrink-0 flex-wrap items-center gap-2 border-b border-border px-3 py-2">
		<a
			href={`/pdf/${id}`}
			class="flex items-center gap-1 rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
			aria-label="Voltar para os detalhes do PDF"
		>
			<i class="bx bx-arrow-back text-lg"></i>
		</a>
		<h1 class="min-w-0 flex-1 truncate text-sm font-semibold">{pdf?.name ?? 'Carregando…'}</h1>

		<div class="flex flex-wrap items-center gap-2">
			<Button
				variant={inverted ? 'secondary' : 'outline'}
				size="sm"
				onclick={toggleInverted}
				aria-pressed={inverted}
			>
				<i class="bx bx-adjust"></i> Inverter cores
			</Button>
			<Button
				variant={keepAwake ? 'secondary' : 'outline'}
				size="sm"
				onclick={toggleKeepAwake}
				disabled={!wakeLockSupported}
				title={wakeLockSupported ? '' : 'Não suportado neste navegador'}
				aria-pressed={keepAwake}
			>
				<i class="bx bx-coffee"></i> Manter tela ligada
			</Button>
		</div>
	</div>

	{#if errorMsg}
		<p class="border-b border-border bg-destructive/10 px-3 py-2 text-sm text-destructive">{errorMsg}</p>
	{/if}

	{#if pdf}
		<div class="min-h-0 flex-1 overflow-hidden">
			<PdfViewer
				src={fileUrl}
				{inverted}
				initialPage={pdf.current_page || 1}
				onready={handleViewerReady}
				onpagechange={handlePageChange}
			/>
		</div>
	{:else if !errorMsg}
		<div class="flex flex-1 items-center justify-center text-sm text-muted-foreground">Carregando…</div>
	{/if}
</div>
