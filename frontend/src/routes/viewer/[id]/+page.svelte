<script lang="ts">
	// Host da ponte postMessage do viewer pdf.js (ver "Contrato: PDF viewer
	// iframe"). Este componente possui: busca de metadados do PDF e das
	// anotações existentes, o <iframe> estático em /pdfjs/viewer.html, os
	// toggles de modo invertido e "manter tela ligada", e o tratamento de
	// toda mensagem vinda do iframe (salvar página atual, criar/atualizar
	// anotação, resolver âncora legada, erro de carregamento).
	import { page } from '$app/state';
	import { apiJSON, apiRequest } from '$lib/api';
	import { extractAssetsFromUrl } from '$lib/pdf-process';
	import { createAnnotation, patchAnnotation } from '$lib/annotations.svelte';
	import { Button } from '$lib/components/ui/button';
	import type { Annotation, PDF } from '$lib/types';

	// page.params tipa como `string | undefined` porque LayoutParams é
	// compartilhado entre todas as rotas de '/' — nesta rota o segmento
	// [id] está sempre presente em tempo de execução.
	const id = $derived(page.params.id ?? '');

	let pdf = $state<PDF | null>(null);
	let inverted = $state(false);
	let keepAwake = $state(false);
	let viewerReady = $state(false);
	let errorMsg = $state('');
	let iframeEl = $state<HTMLIFrameElement>();
	let wakeLock: WakeLockSentinel | null = null;
	let annotations: Annotation[] = [];

	const wakeLockSupported = typeof navigator !== 'undefined' && 'wakeLock' in navigator;

	const viewerSrc = $derived(
		pdf
			? `/pdfjs/viewer.html?file=${encodeURIComponent(`/api/pdfs/${id}/file`)}&readonly=0&page=${pdf.current_page || 1}`
			: ''
	);

	type HostToViewerMessage =
		| { type: 'pdfjs:set-inverted'; value: boolean }
		| { type: 'pdfjs:goto-page'; page: number }
		| { type: 'pdfjs:load-annotations'; items: Annotation[] }
		| { type: 'pdfjs:annotation-created'; annotation: Annotation };

	type ViewerToHostMessage =
		| { type: 'pdfjs:ready'; numPages: number }
		| { type: 'pdfjs:page-changed'; page: number }
		| { type: 'pdfjs:create-comment'; page: number; text: string; note: string; rects: string }
		| { type: 'pdfjs:create-highlight'; page: number; text: string; rects: string; color: string }
		| { type: 'pdfjs:update-annotation'; id: string; note: string }
		| { type: 'pdfjs:anchor-resolved'; id: string; rects: string }
		| { type: 'pdfjs:annotation-click'; id: string }
		| { type: 'pdfjs:error'; message: string };

	function isViewerMessage(data: unknown): data is ViewerToHostMessage {
		return !!data && typeof data === 'object' && typeof (data as { type?: unknown }).type === 'string';
	}

	function postToIframe(msg: HostToViewerMessage) {
		iframeEl?.contentWindow?.postMessage(msg, window.location.origin);
	}

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
		viewerReady = false;
		annotations = [];
		await releaseWakeLock(); // evita vazar o sentinel se o componente for reaproveitado para outro [id]
		if (!pdfId) return;
		try {
			const [pdfData, settings, annotationItems] = await Promise.all([
				apiJSON<PDF>(`/pdfs/${pdfId}`),
				apiJSON<Record<string, string>>('/settings'),
				loadAnnotations(pdfId)
			]);
			pdf = pdfData;
			annotations = annotationItems;
			inverted = settings['viewer.inverted'] === '1';
			keepAwake = settings['viewer.keep_awake'] === '1';
			if (keepAwake) await requestWakeLock();
			backfillAssets(pdfId, pdfData.has_text);
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Falha ao carregar o PDF.';
		}
	}

	/** Extrai texto e um preview em resolução atual no navegador (mesmo
	 * pdf.js do upload) e envia ao servidor: texto para documentos que
	 * chegaram sem ele (import do banco legado, watch-dir), preview sempre
	 * — reprocessa até previews antigos de baixa resolução herdados do
	 * import legado. Melhor-esforço: silencioso em qualquer falha, nunca
	 * bloqueia ou interrompe a abertura do viewer (ver
	 * refatoracao/06-frontend.md, "Degradação graciosa"). */
	async function backfillAssets(pdfId: string, hadText: boolean) {
		try {
			const { text, preview } = await extractAssetsFromUrl(`/api/pdfs/${pdfId}/file`);
			if (!hadText && text) {
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

	async function toggleInverted() {
		inverted = !inverted;
		postToIframe({ type: 'pdfjs:set-inverted', value: inverted });
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

	function handleMessage(event: MessageEvent) {
		if (event.origin !== window.location.origin) return;
		if (!isViewerMessage(event.data)) return;
		const msg = event.data;
		switch (msg.type) {
			case 'pdfjs:ready':
				viewerReady = true;
				// Sincroniza o estado persistido de inversão e as anotações
				// já carregadas assim que o listener do iframe está
				// garantidamente pronto.
				postToIframe({ type: 'pdfjs:set-inverted', value: inverted });
				postToIframe({ type: 'pdfjs:load-annotations', items: annotations });
				break;
			case 'pdfjs:page-changed':
				// Já debounced em 2s pelo iframe — sem debounce adicional aqui.
				apiJSON(`/pdfs/${id}`, { method: 'PATCH', body: { current_page: msg.page } }).catch(() => {});
				break;
			case 'pdfjs:create-comment':
				createAnnotation(id, 'comment', msg.page, msg.text, { note: msg.note, rects: msg.rects })
					.then((created) => {
						annotations = [...annotations, created];
						postToIframe({ type: 'pdfjs:annotation-created', annotation: created });
					})
					.catch((err) => {
						errorMsg = err instanceof Error ? err.message : 'Falha ao criar comentário.';
					});
				break;
			case 'pdfjs:create-highlight':
				createAnnotation(id, 'highlight', msg.page, msg.text, { rects: msg.rects, color: msg.color })
					.then((created) => {
						annotations = [...annotations, created];
						postToIframe({ type: 'pdfjs:annotation-created', annotation: created });
					})
					.catch((err) => {
						errorMsg = err instanceof Error ? err.message : 'Falha ao criar destaque.';
					});
				break;
			case 'pdfjs:update-annotation':
				patchAnnotation(msg.id, { note: msg.note }).catch((err) => {
					errorMsg = err instanceof Error ? err.message : 'Falha ao salvar a nota.';
				});
				break;
			case 'pdfjs:anchor-resolved':
				patchAnnotation(msg.id, { rects: msg.rects }).catch(() => {
					// melhor-esforço — o destaque continua exibido pelo iframe mesmo se a persistência falhar
				});
				break;
			case 'pdfjs:annotation-click':
				// no-op — o popover de leitura é renderizado dentro do próprio iframe
				break;
			case 'pdfjs:error':
				errorMsg = msg.message;
				break;
		}
	}

	$effect(() => {
		window.addEventListener('message', handleMessage);
		return () => {
			window.removeEventListener('message', handleMessage);
			releaseWakeLock();
		};
	});
</script>

<div class="flex h-[calc(100vh-3.5rem)] flex-col">
	<div class="flex flex-wrap items-center gap-2 border-b border-border px-3 py-2">
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
		<iframe bind:this={iframeEl} title="Visualizador de PDF" src={viewerSrc} class="w-full flex-1 border-0"></iframe>
	{:else if !errorMsg}
		<div class="flex flex-1 items-center justify-center text-sm text-muted-foreground">Carregando…</div>
	{/if}
</div>
