<script lang="ts">
	// Chat do documento (ver CONTEXT.md): perguntas sobre o conteúdo do PDF
	// ao modelo de Configurações → IA. Efêmero — o histórico vive só aqui e
	// é reenviado inteiro a cada pergunta (POST /api/pdfs/{id}/chat); o único
	// jeito de guardá-lo é copiá-lo para as Notas.
	import { apiJSON, ApiError } from '$lib/api';
	import { Button } from '$lib/components/ui/button';

	type Exchange = { question: string; answer: string; answer_html: string };

	let { pdfId, onCopyToNotes }: { pdfId: string; onCopyToNotes: (html: string) => void } = $props();

	const MAX_EXCHANGES = 20; // espelha chatMaxExchanges no servidor

	let exchanges = $state<Exchange[]>([]);
	let question = $state('');
	let busy = $state(false);
	let error = $state('');
	let blockedReason = $state('');
	let truncated = $state(false);
	let copied = $state(false);

	async function ask() {
		const q = question.trim();
		if (!q || busy || blockedReason) return;
		busy = true;
		error = '';
		try {
			const res = await apiJSON<{ answer: string; answer_html: string; truncated: boolean }>(
				`/pdfs/${pdfId}/chat`,
				{ method: 'POST', body: { question: q, history: exchanges.map(({ question, answer }) => ({ question, answer })) } }
			);
			exchanges = [...exchanges, { question: q, answer: res.answer, answer_html: res.answer_html }];
			truncated = res.truncated;
			question = '';
			copied = false;
		} catch (err) {
			const msg = err instanceof Error ? err.message : 'Falha ao perguntar';
			// 412 = IA não configurada, 422 = PDF sem texto: nenhuma pergunta vai funcionar.
			if (err instanceof ApiError && (err.status === 412 || err.status === 422)) blockedReason = msg;
			else error = msg;
		} finally {
			busy = false;
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			ask();
		}
	}

	function escapeHTML(s: string): string {
		return s.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);
	}

	function copyToNotes() {
		const date = new Date().toISOString().slice(0, 10);
		const body = exchanges
			.map(
				(x) =>
					`<p><strong>Pergunta:</strong> ${escapeHTML(x.question)}</p><p><strong>Resposta:</strong></p>${x.answer_html}`
			)
			.join('');
		onCopyToNotes(`<h3>Chat do documento — ${date}</h3>${body}`);
		copied = true;
	}

	function newConversation() {
		if (!copied && exchanges.length > 0 && !confirm('Descartar esta conversa? Ela ainda não foi copiada para as Notas.')) return;
		exchanges = [];
		truncated = false;
		copied = false;
		error = '';
	}
</script>

<div class="space-y-2 rounded-lg border border-border p-4">
	<div class="flex items-center justify-between gap-2">
		<h2 class="text-sm font-medium">Chat do documento</h2>
		{#if exchanges.length > 0}
			<div class="flex gap-2">
				<Button variant="outline" size="sm" onclick={newConversation}>
					<i class="bx bx-refresh"></i> Nova conversa
				</Button>
				<Button variant="outline" size="sm" onclick={copyToNotes}>
					<i class="bx bx-note"></i>
					{copied ? 'Copiado' : 'Copiar para Notas'}
				</Button>
			</div>
		{/if}
	</div>

	{#if exchanges.length > 0}
		<div class="max-h-[32rem] space-y-3 overflow-y-auto">
			{#each exchanges as x, i (i)}
				<div class="ml-8 whitespace-pre-wrap rounded-md bg-secondary px-3 py-2 text-sm text-secondary-foreground">{x.question}</div>
				<div
					class="mr-8 rounded-md border border-border px-3 py-2 text-sm [&_ol]:list-decimal [&_ol]:pl-5 [&_p]:mb-2 [&_p:last-child]:mb-0 [&_table]:text-xs [&_td]:border [&_td]:border-border [&_td]:px-1 [&_th]:border [&_th]:border-border [&_th]:px-1 [&_ul]:list-disc [&_ul]:pl-5"
				>
					<!-- answer_html vem sanitizado pelo servidor (security.RenderNotes) -->
					{@html x.answer_html}
				</div>
			{/each}
		</div>
	{/if}
	{#if truncated}
		<p class="text-xs text-muted-foreground">
			O documento é longo: só o começo do texto (400 mil caracteres) foi enviado ao modelo.
		</p>
	{/if}

	{#if blockedReason}
		<p class="text-sm text-destructive">{blockedReason}</p>
	{:else if exchanges.length >= MAX_EXCHANGES}
		<p class="text-sm text-muted-foreground">Conversa longa demais — copie para Notas e recomece.</p>
	{:else}
		<textarea
			bind:value={question}
			onkeydown={onKeydown}
			disabled={busy}
			rows={2}
			maxlength={2000}
			placeholder="Pergunte algo sobre o conteúdo deste PDF (Enter envia, Shift+Enter quebra linha)"
			class="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
		></textarea>
		<div class="flex items-center justify-end gap-2">
			{#if error}<p class="flex-1 text-sm text-destructive">{error}</p>{/if}
			<Button size="sm" onclick={ask} disabled={busy || !question.trim()}>
				<i class="bx bx-send"></i>
				{busy ? 'Pensando…' : 'Perguntar'}
			</Button>
		</div>
	{/if}
</div>
