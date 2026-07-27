<script lang="ts">
	// Página de detalhes do PDF — metadados, ações, notas (TipTap),
	// compartilhamento e anotações do documento (ver refatoracao/06-frontend.md,
	// linha "/pdf/[id]", e refatoracao/10-inventario-funcionalidades.md,
	// "Metadados do documento", "Ações sobre o PDF", "Entrega de arquivo e
	// progresso de leitura", "Anotações", "Compartilhamento e
	// administração").
	import { onDestroy, tick } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { apiJSON, ApiError } from '$lib/api';
	import { AnnotationListStore, deleteAnnotation } from '$lib/annotations.svelte';
	import EmbedButton from '$lib/components/embed-button.svelte';
	import TagPicker from '$lib/components/tag-picker.svelte';
	import ScrollSentinel from '$lib/components/scroll-sentinel.svelte';
	import { Button, buttonVariants } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { formatBytes, formatDate } from '$lib/utils';
	import { LEGACY_HEX } from '$lib/embedpdf';
	import type { PDF, Share } from '$lib/types';
	import { Editor } from '@tiptap/core';
	import StarterKit from '@tiptap/starter-kit';
	import TurndownService from 'turndown';

	const id = $derived(page.params.id ?? '');

	// Mesmo padrão do servidor (ver internal/server/handlers_pdfs.go,
	// fileDirectoryPattern) — a ausência de '.' torna ".." impossível.
	const FILE_DIRECTORY_PATTERN = /^[A-Za-z0-9_\-/]{0,120}$/;
	const turndown = new TurndownService();

	let pdf = $state<PDF | null>(null);
	let notFound = $state(false);
	let loadError = $state('');


	let nameInput = $state('');
	let descriptionInput = $state('');
	let tagsInput = $state('');
	let fileDirectoryInput = $state('');
	let fileDirectoryError = $state('');
	let metaError = $state('');

	let editorEl = $state<HTMLDivElement>();
	let editor: Editor | undefined;
	let notesSaving = $state(false);
	let notesMessage = $state('');
	let notesIsError = $state(false);

	let shareUrl = $state<string | null>(null);
	let shareBusy = $state(false);
	let shareError = $state('');
	let copyMessage = $state('');

	let deleting = $state(false);
	let deleteError = $state('');

	let annotationsError = $state('');
	const highlights = new AnnotationListStore();
	const comments = new AnnotationListStore();

	$effect(() => {
		loadAll(id);
	});

	onDestroy(() => {
		editor?.destroy();
	});

	function syncInputsFromPdf(p: PDF) {
		nameInput = p.name;
		descriptionInput = p.description;
		tagsInput = p.tags.map((t) => t.name).join(' ');
		fileDirectoryInput = p.file_directory;
		fileDirectoryError = '';
	}

	function truncate(text: string, max = 140): string {
		return text.length > max ? `${text.slice(0, max)}…` : text;
	}

	function blurOnEnter(e: KeyboardEvent) {
		if (e.key !== 'Enter') return;
		e.preventDefault();
		(e.currentTarget as HTMLElement).blur();
	}

	async function loadAll(pdfId: string) {
		editor?.destroy();
		editor = undefined;
		pdf = null;
		notFound = false;
		loadError = '';
		shareUrl = null;
		metaError = '';
		deleteError = '';
		shareError = '';
		notesMessage = '';
		annotationsError = '';

		let fetched: PDF;
		try {
			fetched = await apiJSON<PDF>(`/pdfs/${pdfId}`);
		} catch (err) {
			if (err instanceof ApiError && err.status === 404) {
				notFound = true;
			} else {
				loadError = err instanceof Error ? err.message : 'Falha ao carregar o PDF';
			}
			return;
		}

		pdf = fetched;
		syncInputsFromPdf(fetched);

		highlights.pdfId = pdfId;
		highlights.kind = 'highlight';
		comments.pdfId = pdfId;
		comments.kind = 'comment';

		// A div do editor só existe no DOM depois que `pdf` deixa de ser null
		// (ver bloco {:else if pdf} do template) — aguarda o flush reativo.
		await tick();
		if (editorEl) {
			editor = new Editor({ element: editorEl, extensions: [StarterKit], content: fetched.notes_html });
		}


		try {
			const shares = await apiJSON<Share[]>('/shares');
			const existing = shares.find((s) => s.pdf_id === pdfId);
			shareUrl = existing ? `${location.origin}/s/${existing.id}` : null;
		} catch {
			// estado do compartilhamento fica desconhecido — "Compartilhar" ainda funciona
		}

		try {
			await Promise.all([highlights.reset(), comments.reset()]);
		} catch (err) {
			annotationsError = err instanceof Error ? err.message : 'Falha ao carregar anotações';
		}
	}

	/** PATCH genérico — sempre atualiza `pdf` a partir da resposta do servidor,
	 * nunca assume que o corpo enviado voltou inalterado (tags são
	 * renormalizadas no servidor, por exemplo). */
	async function patchPdf(body: Record<string, unknown>): Promise<PDF | null> {
		const current = pdf;
		if (!current) return null;
		metaError = '';
		try {
			const updated = await apiJSON<PDF>(`/pdfs/${current.id}`, { method: 'PATCH', body });
			pdf = updated;
			return updated;
		} catch (err) {
			metaError = err instanceof Error ? err.message : 'Falha ao salvar alteração';
			return null;
		}
	}

	async function saveName() {
		if (!pdf || nameInput === pdf.name) return;
		const updated = await patchPdf({ name: nameInput });
		if (updated) nameInput = updated.name;
	}

	async function saveDescription() {
		if (!pdf || descriptionInput === pdf.description) return;
		const updated = await patchPdf({ description: descriptionInput });
		if (updated) descriptionInput = updated.description;
	}

	async function saveTags(names: string[]) {
		if (!pdf) return;
		tagsInput = names.join(' ');
		const updated = await patchPdf({ tags: names });
		if (updated) tagsInput = updated.tags.map((t) => t.name).join(' ');
	}

	async function saveFileDirectory() {
		const current = pdf;
		if (!current) return;
		if (!FILE_DIRECTORY_PATTERN.test(fileDirectoryInput)) {
			fileDirectoryError = 'Use apenas letras, números, "_", "-" e "/" (máx. 120 caracteres).';
			return;
		}
		fileDirectoryError = '';
		if (fileDirectoryInput === current.file_directory) return;
		const updated = await patchPdf({ file_directory: fileDirectoryInput });
		if (updated) fileDirectoryInput = updated.file_directory;
	}


	async function toggleStarred() {
		if (pdf) await patchPdf({ starred: !pdf.starred });
	}

	async function toggleArchived() {
		if (pdf) await patchPdf({ archived: !pdf.archived });
	}

	async function saveNotes() {
		const current = pdf;
		if (!editor || !current) return;
		notesSaving = true;
		notesMessage = '';
		try {
			const markdown = turndown.turndown(editor.getHTML());
			const updated = await apiJSON<PDF>(`/pdfs/${current.id}`, { method: 'PATCH', body: { notes: markdown } });
			pdf = updated;
			editor.commands.setContent(updated.notes_html);
			notesMessage = 'Notas salvas.';
			notesIsError = false;
		} catch (err) {
			notesMessage = err instanceof Error ? err.message : 'Falha ao salvar notas';
			notesIsError = true;
		} finally {
			notesSaving = false;
		}
	}

	async function handleDelete() {
		const current = pdf;
		if (!current) return;
		if (!confirm(`Excluir "${current.name}"? Esta ação não pode ser desfeita.`)) return;
		deleting = true;
		deleteError = '';
		try {
			await apiJSON(`/pdfs/${current.id}`, { method: 'DELETE' });
			goto('/');
		} catch (err) {
			deleteError = err instanceof Error ? err.message : 'Falha ao excluir';
			deleting = false;
		}
	}

	async function createShare() {
		const current = pdf;
		if (!current) return;
		shareBusy = true;
		shareError = '';
		try {
			const created = await apiJSON<{ id: string; url: string }>(`/pdfs/${current.id}/share`, { method: 'POST' });
			shareUrl = created.url;
		} catch (err) {
			shareError = err instanceof Error ? err.message : 'Falha ao compartilhar';
		} finally {
			shareBusy = false;
		}
	}

	async function revokeShare() {
		const current = pdf;
		if (!current) return;
		if (!confirm('Revogar o link de compartilhamento?')) return;
		shareBusy = true;
		shareError = '';
		try {
			await apiJSON(`/pdfs/${current.id}/share`, { method: 'DELETE' });
			shareUrl = null;
		} catch (err) {
			shareError = err instanceof Error ? err.message : 'Falha ao revogar';
		} finally {
			shareBusy = false;
		}
	}

	async function copyShareUrl() {
		if (!shareUrl) return;
		await navigator.clipboard.writeText(shareUrl);
		copyMessage = 'Link copiado!';
		setTimeout(() => (copyMessage = ''), 2000);
	}

	async function removeAnnotation(store: AnnotationListStore, annotationId: string) {
		if (!confirm('Excluir esta anotação?')) return;
		annotationsError = '';
		try {
			await deleteAnnotation(annotationId);
			store.remove(annotationId);
		} catch (err) {
			annotationsError = err instanceof Error ? err.message : 'Falha ao excluir anotação';
		}
	}
</script>

{#if notFound}
	<div class="mx-auto max-w-2xl p-8 text-center">
		<p class="text-lg font-medium">PDF não encontrado</p>
		<a href="/" class="mt-4 inline-block text-sm text-primary underline-offset-4 hover:underline">← Voltar para a biblioteca</a>
	</div>
{:else if loadError}
	<div class="mx-auto max-w-2xl p-8 text-center">
		<p class="text-sm text-destructive">{loadError}</p>
		<a href="/" class="mt-4 inline-block text-sm text-primary underline-offset-4 hover:underline">← Voltar para a biblioteca</a>
	</div>
{:else if pdf}
	{@const currentPdf = pdf}
	<div class="mx-auto max-w-4xl space-y-6 p-4">
		<a href="/" class="text-sm text-muted-foreground hover:text-foreground">← Biblioteca</a>

		<div class="flex flex-col gap-4 sm:flex-row">
			<img
				src={`/api/pdfs/${currentPdf.id}/preview`}
				alt=""
				class="h-64 w-48 shrink-0 self-start rounded-lg border border-border bg-muted object-cover"
			/>
			<div class="flex-1 space-y-3">
				<Input
					bind:value={nameInput}
					onblur={saveName}
					onkeydown={blurOnEnter}
					placeholder="Nome do PDF"
					aria-label="Nome do PDF"
					class="h-auto px-3 py-2 text-lg font-semibold"
				/>

				<div class="flex flex-wrap items-center gap-2">
					<Button variant={currentPdf.starred ? 'secondary' : 'outline'} size="sm" onclick={toggleStarred}>
						<i class={`bx ${currentPdf.starred ? 'bxs-star text-yellow-500' : 'bx-star'}`}></i>
						{currentPdf.starred ? 'Marcado' : 'Estrela'}
					</Button>
					<Button variant={currentPdf.archived ? 'secondary' : 'outline'} size="sm" onclick={toggleArchived}>
						<i class="bx bx-archive"></i>
						{currentPdf.archived ? 'Arquivado' : 'Arquivar'}
					</Button>
					<EmbedButton pdf={currentPdf} onUpdated={(updated) => (pdf = updated)} showLabel={true} />
				</div>

				<div class="flex flex-wrap items-center gap-2">
					<a href={`/viewer/${currentPdf.id}`} class={buttonVariants({ variant: 'outline', size: 'sm' })}>
						<i class="bx bx-book-open"></i> Abrir no viewer
					</a>
					<a href={`/api/pdfs/${currentPdf.id}/download`} download class={buttonVariants({ variant: 'outline', size: 'sm' })}>
						<i class="bx bx-download"></i> Baixar
					</a>
					<Button variant="destructive" size="sm" onclick={handleDelete} disabled={deleting}>
						<i class="bx bx-trash"></i>
						{deleting ? 'Excluindo…' : 'Excluir'}
					</Button>
				</div>
				{#if deleteError}<p class="text-sm text-destructive">{deleteError}</p>{/if}

				<div class="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
					<span>{currentPdf.num_pages} páginas</span>
					<span>{formatBytes(currentPdf.size_bytes)}</span>
					<span>{currentPdf.views} visualizações</span>
					<span>Enviado em {formatDate(currentPdf.created_at)}</span>
					{#if currentPdf.last_viewed_at}<span>Última leitura em {formatDate(currentPdf.last_viewed_at)}</span>{/if}
				</div>
			</div>
		</div>

		{#if metaError}<p class="text-sm text-destructive">{metaError}</p>{/if}

		<div class="space-y-4 rounded-lg border border-border p-4">
			<div>
				<label for="pdf-description" class="text-sm font-medium">Descrição</label>
				<textarea
					id="pdf-description"
					bind:value={descriptionInput}
					onblur={saveDescription}
					rows={3}
					placeholder="Sem descrição"
					class="mt-1 flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
				></textarea>
			</div>

			<div>
				<label for="pdf-tags" class="text-sm font-medium">Tags</label>
				<div class="mt-1">
					<TagPicker id="pdf-tags" value={tagsInput} onChange={saveTags} />
				</div>
			</div>

			<div>
				<label for="pdf-directory" class="text-sm font-medium">Subdiretório</label>
				<Input
					id="pdf-directory"
					bind:value={fileDirectoryInput}
					onblur={saveFileDirectory}
					onkeydown={blurOnEnter}
					placeholder="ex.: trabalho/2024"
					class="mt-1"
				/>
				{#if fileDirectoryError}<p class="mt-1 text-sm text-destructive">{fileDirectoryError}</p>{/if}
			</div>
		</div>

		<div class="space-y-2 rounded-lg border border-border p-4">
			<div class="flex items-center justify-between">
				<h2 class="text-sm font-medium">Notas</h2>
				<Button size="sm" onclick={saveNotes} disabled={notesSaving}>
					{notesSaving ? 'Salvando…' : 'Salvar notas'}
				</Button>
			</div>
			<div
				bind:this={editorEl}
				class="min-h-32 rounded-md border border-input bg-background px-3 py-2 text-sm [&_.tiptap]:min-h-28 [&_.tiptap]:outline-none [&_p]:mb-2 [&_p:last-child]:mb-0 [&_ul]:mb-2 [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:mb-2 [&_ol]:list-decimal [&_ol]:pl-5 [&_h1]:mb-2 [&_h1]:text-xl [&_h1]:font-bold [&_h2]:mb-2 [&_h2]:text-lg [&_h2]:font-semibold [&_h3]:mb-2 [&_h3]:font-semibold [&_blockquote]:mb-2 [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:py-0.5 [&_code]:text-xs [&_pre]:mb-2 [&_pre]:rounded [&_pre]:bg-muted [&_pre]:p-2 [&_pre_code]:bg-transparent [&_pre_code]:p-0"
			></div>
			{#if notesMessage}
				<p class={notesIsError ? 'text-sm text-destructive' : 'text-sm text-muted-foreground'}>{notesMessage}</p>
			{/if}
		</div>

		<div class="space-y-2 rounded-lg border border-border p-4">
			<h2 class="text-sm font-medium">Compartilhamento</h2>
			{#if shareUrl}
				<div class="flex flex-wrap items-center gap-2">
					<code class="flex-1 truncate rounded bg-secondary px-2 py-1 text-xs text-secondary-foreground">{shareUrl}</code>
					<Button variant="outline" size="sm" onclick={copyShareUrl}>
						<i class="bx bx-copy"></i> Copiar
					</Button>
					<Button variant="destructive" size="sm" onclick={revokeShare} disabled={shareBusy}>
						{shareBusy ? 'Revogando…' : 'Revogar'}
					</Button>
				</div>
				{#if copyMessage}<p class="text-sm text-muted-foreground">{copyMessage}</p>{/if}
			{:else}
				<Button variant="outline" size="sm" onclick={createShare} disabled={shareBusy}>
					<i class="bx bx-share-alt"></i>
					{shareBusy ? 'Compartilhando…' : 'Compartilhar'}
				</Button>
			{/if}
			{#if shareError}<p class="text-sm text-destructive">{shareError}</p>{/if}
		</div>

		{#if annotationsError}<p class="text-sm text-destructive">{annotationsError}</p>{/if}
		<div class="grid gap-4 sm:grid-cols-2">
			<div class="rounded-lg border border-border p-4">
				<h2 class="mb-2 text-sm font-medium">Destaques do PDF</h2>
				{#if highlights.items.length === 0 && !highlights.loading}
					<p class="text-sm text-muted-foreground">Nenhum destaque ainda.</p>
				{:else}
					<ul class="space-y-2">
						{#each highlights.items as ann (ann.id)}
							<li class="flex items-start justify-between gap-2 text-sm">
								<p class="flex min-w-0 flex-1 items-start gap-1.5">
									<span
										class="mt-1.5 h-2 w-2 shrink-0 rounded-full"
										style:background-color={LEGACY_HEX[ann.color] ?? ann.color}
									></span>
									<span class="min-w-0">
										<span class="font-medium">p. {ann.page}</span>
										<span class="text-muted-foreground">— {truncate(ann.text)}</span>
										{#if ann.note}<span class="block text-xs italic text-muted-foreground">{truncate(ann.note)}</span>{/if}
									</span>
								</p>
								<button
									type="button"
									class="shrink-0 text-muted-foreground hover:text-destructive"
									onclick={() => removeAnnotation(highlights, ann.id)}
									aria-label="Excluir destaque"
								>
									<i class="bx bx-trash"></i>
								</button>
							</li>
						{/each}
					</ul>
				{/if}
				<ScrollSentinel onIntersect={() => highlights.loadMore()} disabled={highlights.done} />
				{#if highlights.loading}<p class="text-xs text-muted-foreground">Carregando…</p>{/if}
			</div>

			<div class="rounded-lg border border-border p-4">
				<h2 class="mb-2 text-sm font-medium">Comentários do PDF</h2>
				{#if comments.items.length === 0 && !comments.loading}
					<p class="text-sm text-muted-foreground">Nenhum comentário ainda.</p>
				{:else}
					<ul class="space-y-2">
						{#each comments.items as ann (ann.id)}
							<li class="flex items-start justify-between gap-2 text-sm">
								<p class="flex min-w-0 flex-1 items-start gap-1.5">
									<span
										class="mt-1.5 h-2 w-2 shrink-0 rounded-full"
										style:background-color={LEGACY_HEX[ann.color] ?? ann.color}
									></span>
									<span class="min-w-0">
										<span class="font-medium">p. {ann.page}</span>
										<span class="text-muted-foreground">— {truncate(ann.text)}</span>
										{#if ann.note}<span class="block text-xs italic text-muted-foreground">{truncate(ann.note)}</span>{/if}
									</span>
								</p>
								<button
									type="button"
									class="shrink-0 text-muted-foreground hover:text-destructive"
									onclick={() => removeAnnotation(comments, ann.id)}
									aria-label="Excluir comentário"
								>
									<i class="bx bx-trash"></i>
								</button>
							</li>
						{/each}
					</ul>
				{/if}
				<ScrollSentinel onIntersect={() => comments.loadMore()} disabled={comments.done} />
				{#if comments.loading}<p class="text-xs text-muted-foreground">Carregando…</p>{/if}
			</div>
		</div>
	</div>
{:else}
	<p class="p-8 text-center text-sm text-muted-foreground">Carregando…</p>
{/if}
