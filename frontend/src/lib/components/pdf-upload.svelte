<script lang="ts">
	import { apiJSON, ApiError } from '$lib/api';
	import { processPDF } from '$lib/pdf-process';
	import type { PDF } from '$lib/types';

	let { onUploaded }: { onUploaded: (pdf: PDF) => void } = $props();

	let uploading = $state(false);
	let error = $state('');
	let fileInput = $state<HTMLInputElement>();

	async function handleFiles(files: FileList | null) {
		if (!files || files.length === 0) return;
		error = '';
		for (const file of Array.from(files)) {
			await uploadOne(file);
		}
		if (fileInput) fileInput.value = '';
	}

	async function uploadOne(file: File) {
		uploading = true;
		try {
			const form = new FormData();
			form.append('file', file);
			form.append('name', file.name.replace(/\.pdf$/i, ''));

			try {
				const processed = await processPDF(file);
				form.append('preview', processed.preview, 'preview.png');
				form.append('text', processed.text);
				form.append('num_pages', String(processed.numPages));
			} catch {
				// Degradação graciosa: pdf.js falhou (PDF corrompido/protegido) —
				// envia só o arquivo original (ver 06-frontend.md).
			}

			const pdf = await apiJSON<PDF>('/pdfs', { method: 'POST', body: form });
			onUploaded(pdf);
		} catch (err) {
			if (err instanceof ApiError && err.status === 409) {
				error = `"${file.name}" já existe no acervo.`;
			} else {
				error = err instanceof Error ? err.message : 'Falha no upload';
			}
		} finally {
			uploading = false;
		}
	}

	function handleDrop(e: DragEvent) {
		e.preventDefault();
		handleFiles(e.dataTransfer?.files ?? null);
	}
</script>

<div
	class="rounded-lg border-2 border-dashed border-border p-6 text-center"
	role="button"
	tabindex="0"
	ondrop={handleDrop}
	ondragover={(e) => e.preventDefault()}
>
	<input
		bind:this={fileInput}
		type="file"
		accept="application/pdf"
		multiple
		class="hidden"
		id="pdf-upload-input"
		onchange={(e) => handleFiles((e.target as HTMLInputElement).files)}
	/>
	<label for="pdf-upload-input" class="cursor-pointer">
		<i class="bx bx-upload text-2xl"></i>
		<p class="text-sm text-muted-foreground">
			{uploading ? 'Enviando…' : 'Arraste PDFs aqui ou clique para enviar'}
		</p>
	</label>
	{#if error}
		<p class="mt-2 text-sm text-destructive">{error}</p>
	{/if}
</div>
