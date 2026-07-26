// Visualizador estático de pdf.js — módulo ES nativo do navegador, sem
// bundler processando este arquivo, carregado dentro do <iframe> das rotas
// /viewer/[id] e /s/[share]. Renderiza cada página em um <canvas> próprio
// (mesmo padrão de frontend/src/lib/pdf-process.ts) em vez do widget
// PDFViewer de pdf_viewer.mjs — simples o bastante para uma leitura com
// rolagem contínua. Cada página também recebe uma TextLayer (classe
// exportada por pdf.mjs) invisível sobreposta ao canvas, só para permitir
// seleção nativa de texto (necessária para o destaque). Contrato completo
// de query params e ponte postMessage: ver "Contrato: PDF viewer iframe"
// no plano da ETAPA-10.

import * as pdfjsLib from './pdf.mjs';

pdfjsLib.GlobalWorkerOptions.workerSrc = './pdf.worker.mjs';

const params = new URLSearchParams(location.search);
const fileUrl = params.get('file');
const readonly = params.get('readonly') === '1';
const initialPage = Math.max(1, parseInt(params.get('page') ?? '1', 10) || 1);

const pageIndicator = document.getElementById('page-indicator');
const commentBtn = document.getElementById('comment-btn');
const viewerContainer = document.getElementById('viewerContainer');
const pagesContainer = document.getElementById('pagesContainer');
const selectionToolbar = document.getElementById('selection-toolbar');
const highlightBtn = document.getElementById('highlight-btn');
const statusEl = document.getElementById('status');

const pageElements = new Map(); // page number -> wrapper <div>
const visibleRatios = new Map(); // page number -> última intersectionRatio conhecida
let currentPage = initialPage;
let numPages = 0;
let pageChangeTimer = null;

function post(message) {
	window.parent.postMessage(message, location.origin);
}

function showStatus(message, isError) {
	statusEl.textContent = message;
	statusEl.classList.toggle('error', !!isError);
	statusEl.classList.remove('hidden');
}

function updateIndicator() {
	pageIndicator.textContent = numPages ? `Página ${currentPage} de ${numPages}` : 'Carregando…';
}

function schedulePageChange(targetPage) {
	if (readonly) return; // salvar página é uma ação de escrita — indisponível em modo somente-leitura
	clearTimeout(pageChangeTimer);
	pageChangeTimer = setTimeout(() => post({ type: 'pdfjs:page-changed', page: targetPage }), 2000);
}

function scrollToPage(targetPage, behavior = 'smooth') {
	const el = pageElements.get(targetPage);
	if (el) el.scrollIntoView({ behavior, block: 'start' });
}

// Um único IntersectionObserver observa todas as páginas; mantém a última
// ratio conhecida de cada uma (não só as do lote de entries do callback)
// para sempre escolher a página com maior visibilidade real, não apenas a
// que mudou por último.
function observePages() {
	const observer = new IntersectionObserver(
		(entries) => {
			for (const entry of entries) {
				const num = Number(entry.target.dataset.page);
				visibleRatios.set(num, entry.isIntersecting ? entry.intersectionRatio : 0);
			}
			let bestPage = currentPage;
			let bestRatio = 0;
			for (const [num, ratio] of visibleRatios) {
				if (ratio > bestRatio) {
					bestRatio = ratio;
					bestPage = num;
				}
			}
			if (bestRatio > 0 && bestPage !== currentPage) {
				currentPage = bestPage;
				updateIndicator();
				schedulePageChange(currentPage);
			}
		},
		{ root: viewerContainer, threshold: [0, 0.1, 0.25, 0.5, 0.75, 1] }
	);
	for (const el of pageElements.values()) observer.observe(el);
}

async function renderPDF() {
	if (!fileUrl) {
		const message = 'Parâmetro "file" ausente na URL do visualizador.';
		showStatus(message, true);
		post({ type: 'pdfjs:error', message });
		return;
	}

	try {
		const res = await fetch(fileUrl, { credentials: 'same-origin' });
		if (!res.ok) throw new Error(`Falha ao baixar o PDF (HTTP ${res.status})`);
		const data = await res.arrayBuffer();

		const doc = await pdfjsLib.getDocument({
			data,
			cMapUrl: './cmaps/',
			cMapPacked: true,
			standardFontDataUrl: './standard_fonts/'
		}).promise;

		numPages = doc.numPages;
		if (currentPage > numPages) currentPage = numPages || 1;
		updateIndicator();
		post({ type: 'pdfjs:ready', numPages });

		const containerWidth = viewerContainer.clientWidth || 800;
		for (let num = 1; num <= doc.numPages; num++) {
			const pdfPage = await doc.getPage(num);
			const unscaled = pdfPage.getViewport({ scale: 1 });
			const scale = Math.min(2.2, Math.max(0.4, (containerWidth - 32) / unscaled.width));
			const viewport = pdfPage.getViewport({ scale });

			const wrap = document.createElement('div');
			wrap.className = 'page-wrap';
			wrap.dataset.page = String(num);
			wrap.style.setProperty('--scale-factor', String(scale));

			const canvas = document.createElement('canvas');
			canvas.className = 'pdf-page';
			canvas.width = viewport.width;
			canvas.height = viewport.height;
			wrap.appendChild(canvas);

			const textLayerDiv = document.createElement('div');
			textLayerDiv.className = 'textLayer';
			wrap.appendChild(textLayerDiv);

			pagesContainer.appendChild(wrap);
			pageElements.set(num, wrap);

			const ctx = canvas.getContext('2d');
			const textLayer = new pdfjsLib.TextLayer({
				textContentSource: pdfPage.streamTextContent(),
				container: textLayerDiv,
				viewport
			});
			await Promise.all([pdfPage.render({ canvasContext: ctx, viewport, canvas }).promise, textLayer.render()]);
		}

		observePages();
		scrollToPage(initialPage, 'auto');
	} catch (err) {
		const message = err instanceof Error ? err.message : 'Falha ao carregar o PDF.';
		showStatus(message, true);
		post({ type: 'pdfjs:error', message });
	}
}

function applySignature(dataUrl) {
	const wrap = pageElements.get(currentPage);
	if (!wrap || !dataUrl) return;
	const existing = wrap.querySelector('.signature-overlay');
	if (existing) existing.remove();
	const img = document.createElement('img');
	img.className = 'signature-overlay';
	img.src = dataUrl;
	img.alt = 'Assinatura';
	wrap.appendChild(img);
}

// --- Barra de ferramentas de autoria (somente quando !readonly) ---

if (readonly) {
	commentBtn.remove();
} else {
	commentBtn.addEventListener('click', () => {
		const text = window.prompt('Texto do comentário:');
		if (text && text.trim()) post({ type: 'pdfjs:create-comment', page: currentPage, text: text.trim() });
	});

	document.addEventListener('mouseup', (event) => {
		if (selectionToolbar.contains(event.target)) return; // clique no próprio botão "Destacar"
		const selection = window.getSelection();
		const text = selection ? selection.toString().trim() : '';
		if (!text || !selection.rangeCount) {
			selectionToolbar.classList.add('hidden');
			return;
		}
		const rect = selection.getRangeAt(0).getBoundingClientRect();
		selectionToolbar.style.top = `${window.scrollY + rect.top - 40}px`;
		selectionToolbar.style.left = `${window.scrollX + rect.left}px`;
		selectionToolbar.dataset.text = text;
		selectionToolbar.classList.remove('hidden');
	});

	document.addEventListener('mousedown', (event) => {
		if (!selectionToolbar.contains(event.target)) selectionToolbar.classList.add('hidden');
	});

	highlightBtn.addEventListener('click', () => {
		const text = selectionToolbar.dataset.text || '';
		if (text) post({ type: 'pdfjs:create-highlight', page: currentPage, text });
		selectionToolbar.classList.add('hidden');
		window.getSelection()?.removeAllRanges();
	});
}

// --- Ponte host → iframe ---

window.addEventListener('message', (event) => {
	if (event.origin !== location.origin) return;
	const msg = event.data;
	if (!msg || typeof msg !== 'object') return;

	if (msg.type === 'pdfjs:apply-signature') {
		if (!readonly) applySignature(msg.dataUrl); // aplicar assinatura é uma ação de escrita
	} else if (msg.type === 'pdfjs:set-inverted') {
		pagesContainer.classList.toggle('inverted', !!msg.value);
	} else if (msg.type === 'pdfjs:goto-page') {
		scrollToPage(Number(msg.page));
	}
});

renderPDF();
