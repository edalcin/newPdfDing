// Processamento de PDF no navegador antes do upload (ver
// refatoracao/06-frontend.md, "Upload — processamento no navegador"):
// contagem de páginas, thumbnail (400px) + preview (1000px) renderizados da
// página 1, e texto extraído de todas as páginas (limitado a 2 MB).

import * as pdfjsLib from 'pdfjs-dist';
import type { PDFPageProxy } from 'pdfjs-dist';

pdfjsLib.GlobalWorkerOptions.workerSrc = new URL('pdfjs-dist/build/pdf.worker.mjs', import.meta.url).href;

const TEXT_LIMIT_BYTES = 2 * 1024 * 1024;

export interface ProcessedPDF {
	numPages: number;
	thumbnail: Blob;
	preview: Blob;
	text: string;
}

async function renderPageToBlob(page: PDFPageProxy, targetWidth: number): Promise<Blob> {
	const baseViewport = page.getViewport({ scale: 1 });
	const scaledViewport = page.getViewport({ scale: targetWidth / baseViewport.width });

	const canvas = document.createElement('canvas');
	canvas.width = scaledViewport.width;
	canvas.height = scaledViewport.height;
	const ctx = canvas.getContext('2d');
	if (!ctx) throw new Error('canvas 2d context unavailable');

	await page.render({ canvasContext: ctx, viewport: scaledViewport, canvas }).promise;

	const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/png'));
	if (!blob) throw new Error('canvas toBlob failed');
	return blob;
}

/** Runs pdf.js client-side processing. Throws on corrupted/password-protected
 * PDFs — the caller degrades gracefully by uploading the raw file alone
 * (ver 06-frontend.md, "Degradação graciosa"). */
export async function processPDF(file: File): Promise<ProcessedPDF> {
	const buffer = await file.arrayBuffer();
	const doc = await pdfjsLib.getDocument({ data: buffer }).promise;

	const page1 = await doc.getPage(1);
	const [thumbnail, preview] = await Promise.all([renderPageToBlob(page1, 400), renderPageToBlob(page1, 1000)]);

	let text = '';
	for (let pageNum = 1; pageNum <= doc.numPages; pageNum++) {
		const page = await doc.getPage(pageNum);
		const content = await page.getTextContent();
		const pageText = content.items.map((item) => ('str' in item ? item.str : '')).join(' ');
		text += pageText + '\n';
		if (new Blob([text]).size > TEXT_LIMIT_BYTES) break;
	}
	if (new Blob([text]).size > TEXT_LIMIT_BYTES) {
		text = new TextDecoder().decode(new TextEncoder().encode(text).slice(0, TEXT_LIMIT_BYTES));
	}

	return { numPages: doc.numPages, thumbnail, preview, text };
}
