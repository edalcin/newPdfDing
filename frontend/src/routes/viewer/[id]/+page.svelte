<script lang="ts">
	// Host da ponte postMessage do viewer pdf.js (ver "Contrato: PDF viewer
	// iframe"). Este componente possui: busca de metadados do PDF e das
	// assinaturas, o <iframe> estático em /pdfjs/viewer.html, os toggles de
	// modo invertido e "manter tela ligada", o seletor de assinatura, e o
	// tratamento de toda mensagem vinda do iframe (salvar página atual,
	// criar comentário/destaque, erro de carregamento).
	import { page } from '$app/state';
	import { apiJSON } from '$lib/api';
	import { extractTextFromUrl } from '$lib/pdf-process';
	import { createAnnotation } from '$lib/annotations.svelte';
	import { Button } from '$lib/components/ui/button';
	import type { PDF, Signature } from '$lib/types';

	// page.params tipa como `string | undefined` porque LayoutParams é
	// compartilhado entre todas as rotas de '/' — nesta rota o segmento
	// [id] está sempre presente em tempo de execução.
	const id = $derived(page.params.id ?? '');

	let pdf = $state<PDF | null>(null);
	let signatures = $state<Signature[]>([]);
	let selectedSignatureId = $state('');
	let inverted = $state(false);
	let keepAwake = $state(false);
	let viewerReady = $state(false);
	let errorMsg = $state('');
	let iframeEl = $state<HTMLIFrameElement>();
	let wakeLock: WakeLockSentinel | null = null;

	const wakeLockSupported = typeof navigator !== 'undefined' && 'wakeLock' in navigator;

	const viewerSrc = $derived(
		pdf
			? `/pdfjs/viewer.html?file=${encodeURIComponent(`/api/pdfs/${id}/file`)}&readonly=0&page=${pdf.current_page || 1}`
			: ''
	);

	type HostToViewerMessage =
		| { type: 'pdfjs:apply-signature'; dataUrl: string }
		| { type: 'pdfjs:set-inverted'; value: boolean }
		| { type: 'pdfjs:goto-page'; page: number };

	type ViewerToHostMessage =
		| { type: 'pdfjs:ready'; numPages: number }
		| { type: 'pdfjs:page-changed'; page: number }
		| { type: 'pdfjs:create-comment'; page: number; text: string }
		| { type: 'pdfjs:create-highlight'; page: number; text: string }
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

	async function loadData(pdfId: string) {
		pdf = null;
		errorMsg = '';
		viewerReady = false;
		await releaseWakeLock(); // evita vazar o sentinel se o componente for reaproveitado para outro [id]
		if (!pdfId) return;
		try {
			const [pdfData, sigData, settings] = await Promise.all([
				apiJSON<PDF>(`/pdfs/${pdfId}`),
				apiJSON<Signature[]>('/signatures'),
				apiJSON<Record<string, string>>('/settings')
			]);
			pdf = pdfData;
			signatures = sigData;
			selectedSignatureId = sigData.length > 0 ? sigData[0].id : '';
			inverted = settings['viewer.inverted'] === '1';
			keepAwake = settings['viewer.keep_awake'] === '1';
			if (keepAwake) await requestWakeLock();
			if (!pdfData.has_text) backfillText(pdfId);
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : 'Falha ao carregar o PDF.';
		}
	}

	/** Extrai o texto no navegador (mesmo pdf.js do upload) e envia ao
	 * servidor — para documentos que chegaram sem texto (import do banco
	 * legado, watch-dir). Melhor-esforço: silencioso em qualquer falha,
	 * nunca bloqueia ou interrompe a abertura do viewer (ver
	 * refatoracao/06-frontend.md, "Degradação graciosa"). */
	async function backfillText(pdfId: string) {
		try {
			const text = await extractTextFromUrl(`/api/pdfs/${pdfId}/file`);
			if (!text) return;
			await apiJSON(`/pdfs/${pdfId}/text`, { method: 'POST', body: { text } });
			if (pdf && pdf.id === pdfId) pdf = { ...pdf, has_text: true };
		} catch {
			// PDF corrompido/protegido, ou request falhou — sem texto extraído
			// desta vez; a próxima abertura tenta de novo.
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

	function applySelectedSignature() {
		const sig = signatures.find((s) => s.id === selectedSignatureId);
		if (sig) postToIframe({ type: 'pdfjs:apply-signature', dataUrl: sig.data });
	}

	function handleMessage(event: MessageEvent) {
		if (event.origin !== window.location.origin) return;
		if (!isViewerMessage(event.data)) return;
		const msg = event.data;
		switch (msg.type) {
			case 'pdfjs:ready':
				viewerReady = true;
				// Sincroniza o estado persistido de inversão assim que o
				// listener do iframe está garantidamente pronto.
				postToIframe({ type: 'pdfjs:set-inverted', value: inverted });
				break;
			case 'pdfjs:page-changed':
				// Já debounced em 2s pelo iframe — sem debounce adicional aqui.
				apiJSON(`/pdfs/${id}`, { method: 'PATCH', body: { current_page: msg.page } }).catch(() => {});
				break;
			case 'pdfjs:create-comment':
				createAnnotation(id, 'comment', msg.page, msg.text).catch((err) => {
					errorMsg = err instanceof Error ? err.message : 'Falha ao criar comentário.';
				});
				break;
			case 'pdfjs:create-highlight':
				createAnnotation(id, 'highlight', msg.page, msg.text).catch((err) => {
					errorMsg = err instanceof Error ? err.message : 'Falha ao criar destaque.';
				});
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
			{#if signatures.length > 0}
				<select
					bind:value={selectedSignatureId}
					class="h-9 rounded-md border border-input bg-background px-2 text-sm"
					aria-label="Assinatura"
				>
					{#each signatures as sig (sig.id)}
						<option value={sig.id}>{sig.name}</option>
					{/each}
				</select>
				<Button variant="outline" size="sm" onclick={applySelectedSignature} disabled={!selectedSignatureId || !viewerReady}>
					Aplicar assinatura
				</Button>
			{/if}
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
