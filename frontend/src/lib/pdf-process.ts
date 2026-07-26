// Processamento de PDF no navegador antes do upload (ver
// refatoracao/06-frontend.md, "Upload — processamento no navegador"):
// contagem de páginas, preview renderizado da página 1, e texto extraído
// de todas as páginas (limitado a 2 MB).

import * as pdfjsLib from 'pdfjs-dist';
import type { PDFPageProxy } from 'pdfjs-dist';

pdfjsLib.GlobalWorkerOptions.workerSrc = new URL('pdfjs-dist/build/pdf.worker.mjs', import.meta.url).href;

const TEXT_LIMIT_BYTES = 2 * 1024 * 1024;

// Largura-base em pixels CSS do preview (também usado como miniatura na
// biblioteca — não há mais um asset de thumbnail separado).
const PREVIEW_WIDTH = 1000;

export interface ProcessedPDF {
	numPages: number;
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
	const [preview, text] = await Promise.all([renderPageToBlob(page1, PREVIEW_WIDTH), extractText(doc)]);

	return { numPages: doc.numPages, preview, text };
}

/** Shared text-extraction loop (used by processPDF and extractAssetsFromUrl):
 * walks every page's text content up to TEXT_LIMIT_BYTES. */
async function extractText(doc: pdfjsLib.PDFDocumentProxy): Promise<string> {
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
	return text;
}

export interface BackfilledAssets {
	text: string;
	preview: Blob;
}

/** Extracts text and re-renders the preview for a PDF already stored
 * server-side, fetched by URL once — used by the viewer to backfill
 * `pdf_text` for documents that arrived without it (legacy import,
 * watch-dir consumer's pure-Go extraction gap; ver 05-api.md, "POST
 * .../text") and to refresh a low-resolution preview inherited from a
 * legacy import with the same rendering used at upload time (ver
 * PREVIEW_WIDTH above). Throws on a corrupted/password-protected PDF; the
 * caller treats that as best-effort and ignores the failure. */
export async function extractAssetsFromUrl(url: string): Promise<BackfilledAssets> {
	const res = await fetch(url, { credentials: 'same-origin' });
	if (!res.ok) throw new Error(`failed to fetch pdf: ${res.status}`);
	const buffer = await res.arrayBuffer();
	const doc = await pdfjsLib.getDocument({ data: buffer }).promise;
	const page1 = await doc.getPage(1);
	const [text, preview] = await Promise.all([extractText(doc), renderPageToBlob(page1, PREVIEW_WIDTH)]);
	return { text, preview };
}
