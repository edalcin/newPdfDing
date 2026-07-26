// Visualizador estático de pdf.js — módulo ES nativo do navegador, sem
// bundler processando este arquivo, carregado dentro do <iframe> das rotas
// /viewer/[id] e /s/[share]. Renderiza cada página em um <canvas> próprio
// (mesmo padrão de frontend/src/lib/pdf-process.ts) em vez do widget
// PDFViewer de pdf_viewer.mjs — simples o bastante para uma leitura com
// rolagem contínua. Cada página recebe uma TextLayer (invisível, só para
// seleção nativa) e uma highlightLayer (destaques/comentários ancorados,
// abaixo da TextLayer para não bloquear a seleção). Contrato completo de
// query params e ponte postMessage: ver "Contrato: PDF viewer iframe".

import * as pdfjsLib from './pdf.mjs';

pdfjsLib.GlobalWorkerOptions.workerSrc = './pdf.worker.mjs';

const params = new URLSearchParams(location.search);
const fileUrl = params.get('file');
const readonly = params.get('readonly') === '1';
const initialPage = Math.max(1, parseInt(params.get('page') ?? '1', 10) || 1);

const pageInput = document.getElementById('page-input');
const pageCount = document.getElementById('page-count');
const commentBtn = document.getElementById('comment-btn');
const zoomOutBtn = document.getElementById('zoom-out');
const zoomInBtn = document.getElementById('zoom-in');
const zoomFitBtn = document.getElementById('zoom-fit');
const zoomLevel = document.getElementById('zoom-level');
const outlineBtn = document.getElementById('outline-btn');
const outlinePanel = document.getElementById('outline-panel');
const viewerContainer = document.getElementById('viewerContainer');
const pagesContainer = document.getElementById('pagesContainer');
const selectionToolbar = document.getElementById('selection-toolbar');
const highlightBtn = document.getElementById('highlight-btn');
const commentSelectionBtn = document.getElementById('comment-selection-btn');
const colorButtons = Array.from(document.querySelectorAll('.color-btn'));
const notePopover = document.getElementById('note-popover');
const noteTextarea = document.getElementById('note-textarea');
const noteSaveBtn = document.getElementById('note-save');
const noteCancelBtn = document.getElementById('note-cancel');
const statusEl = document.getElementById('status');

const ZOOM_STEPS = [0.5, 0.75, 1, 1.25, 1.5, 2, 3, 4];

const pageElements = new Map(); // page number -> wrapper <div>
const pageProxies = new Map(); // page number -> pdf.js page proxy
const visibleRatios = new Map(); // page number -> última intersectionRatio conhecida
const annotationsByPage = new Map(); // page number -> annotation[]
const annotationsById = new Map(); // annotation id -> annotation

let currentPage = initialPage;
let numPages = 0;
let pageChangeTimer = null;
let scaleMode = 'fit-width'; // 'fit-width' | number (fator literal de escala)
let selectedColor = 'yellow';
let pendingComment = null; // { text, rects } enquanto o popover de comentário ancorado está aberto
let editingAnnotationId = null; // id da anotação em edição no popover de leitura

function post(message) {
	window.parent.postMessage(message, location.origin);
}

function showStatus(message, isError) {
	statusEl.textContent = message;
	statusEl.classList.toggle('error', !!isError);
	statusEl.classList.remove('hidden');
}

function updateIndicator() {
	if (pageInput) pageInput.value = String(currentPage);
	if (pageCount) pageCount.textContent = numPages ? `/ ${numPages}` : '/ …';
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

function computeScale(pdfPage) {
	if (typeof scaleMode === 'number') return scaleMode;
	const unscaled = pdfPage.getViewport({ scale: 1 });
	const containerWidth = viewerContainer.clientWidth || 800;
	return Math.min(2.2, Math.max(0.4, (containerWidth - 32) / unscaled.width));
}

function updateZoomLevel(scale) {
	if (zoomLevel) zoomLevel.textContent = `${Math.round(scale * 100)}%`;
}

/** Renders (or re-renders) a single page's canvas + TextLayer at the
 * current scale, then redraws its highlight layer (rects are normalized
 * 0..1 so they never need recomputing across zoom levels). */
async function renderPage(num) {
	const pdfPage = pageProxies.get(num);
	const wrap = pageElements.get(num);
	if (!pdfPage || !wrap) return;

	const scale = computeScale(pdfPage);
	const viewport = pdfPage.getViewport({ scale });
	wrap.style.setProperty('--scale-factor', String(scale));

	const canvas = wrap.querySelector('canvas.pdf-page');
	canvas.width = viewport.width;
	canvas.height = viewport.height;

	const textLayerDiv = wrap.querySelector('.textLayer');
	textLayerDiv.replaceChildren();

	const ctx = canvas.getContext('2d');
	const textLayer = new pdfjsLib.TextLayer({
		textContentSource: pdfPage.streamTextContent(),
		container: textLayerDiv,
		viewport
	});
	await Promise.all([pdfPage.render({ canvasContext: ctx, viewport, canvas }).promise, textLayer.render()]);

	renderHighlights(num);
	if (num === currentPage) updateZoomLevel(scale);
	await resolveUnanchoredHighlights(num);
}

async function renderAllPages() {
	for (const num of pageElements.keys()) {
		await renderPage(num);
	}
}

async function setZoom(next) {
	scaleMode = next;
	await renderAllPages();
	scrollToPage(currentPage, 'auto');
}

function nearestStepIndex(scale) {
	let best = 0;
	let bestDiff = Infinity;
	ZOOM_STEPS.forEach((step, i) => {
		const diff = Math.abs(step - scale);
		if (diff < bestDiff) {
			bestDiff = diff;
			best = i;
		}
	});
	return best;
}

function currentNumericScale() {
	if (typeof scaleMode === 'number') return scaleMode;
	const pdfPage = pageProxies.get(currentPage);
	return pdfPage ? computeScale(pdfPage) : 1;
}

function zoomBy(delta) {
	const idx = nearestStepIndex(currentNumericScale());
	const nextIdx = Math.min(ZOOM_STEPS.length - 1, Math.max(0, idx + delta));
	setZoom(ZOOM_STEPS[nextIdx]);
}

// --- Camada de destaques (highlightLayer) ---

function renderHighlights(pageNum) {
	const wrap = pageElements.get(pageNum);
	if (!wrap) return;
	const layer = wrap.querySelector('.highlightLayer');
	layer.replaceChildren();
	const items = annotationsByPage.get(pageNum) || [];
	for (const item of items) {
		if (!item.rects) continue;
		let rects;
		try {
			rects = JSON.parse(item.rects);
		} catch {
			continue;
		}
		for (const [x, y, w, h] of rects) {
			const hl = document.createElement('div');
			hl.className = `hl hl-${item.color || 'yellow'}`;
			hl.style.left = `${x * 100}%`;
			hl.style.top = `${y * 100}%`;
			hl.style.width = `${w * 100}%`;
			hl.style.height = `${h * 100}%`;
			hl.dataset.annotationId = item.id;
			hl.addEventListener('click', () => onHighlightClick(item.id));
			layer.appendChild(hl);
		}
	}
}

function onHighlightClick(id) {
	post({ type: 'pdfjs:annotation-click', id });
	if (readonly) return;
	const item = annotationsById.get(id);
	if (!item) return;
	openPopover('read', { id: item.id, note: item.note || '' });
}

// --- Recuperação de destaques legados sem geometria (rects === '') ---

function normalizeForSearch(text) {
	return text
		.normalize('NFKD')
		.replace(/[\u0300-\u036f]/g, '')
		.replace(/[-\u00ad]/g, '')
		.replace(/\s+/g, ' ')
		.trim()
		.toLowerCase();
}

async function resolveUnanchoredHighlights(pageNum) {
	if (readonly) return; // recuperação grava rects via PATCH — ação de escrita
	const items = (annotationsByPage.get(pageNum) || []).filter(
		(a) => a.kind === 'highlight' && !a.rects && a.text
	);
	if (items.length === 0) return;

	const wrap = pageElements.get(pageNum);
	if (!wrap) return; // página ainda não renderizada — renderPage() a chamará de novo quando terminar
	const textLayerDiv = wrap.querySelector('.textLayer');
	if (!textLayerDiv) return;
	const spans = Array.from(textLayerDiv.querySelectorAll('span'));
	if (spans.length === 0) return;

	// Concatena o texto normalizado de todos os spans, guardando o offset
	// de início de cada um no texto concatenado — permite localizar quais
	// spans compõem um trecho procurado e construir um Range sobre eles.
	let concatenated = '';
	const spanOffsets = [];
	for (const span of spans) {
		spanOffsets.push({ span, start: concatenated.length, text: normalizeForSearch(span.textContent || '') });
		concatenated += spanOffsets[spanOffsets.length - 1].text + ' ';
	}

	for (const item of items) {
		const needle = normalizeForSearch(item.text);
		if (!needle) continue;
		const idx = concatenated.indexOf(needle);
		if (idx === -1) continue; // trecho não encontrado — anotação segue listada sem destaque, nunca apagada

		const endIdx = idx + needle.length;
		const matchingSpans = spanOffsets.filter((s) => s.start < endIdx && s.start + s.text.length > idx).map((s) => s.span);
		if (matchingSpans.length === 0) continue;

		try {
			const range = document.createRange();
			range.setStart(matchingSpans[0], 0);
			const last = matchingSpans[matchingSpans.length - 1];
			range.setEnd(last, last.childNodes.length ? last.childNodes.length : 0);
			const rects = clientRectsToNormalized(range.getClientRects(), wrap);
			if (rects.length === 0) continue;
			const rectsJson = JSON.stringify(rects);
			item.rects = rectsJson;
			annotationsById.set(item.id, item);
			renderHighlights(pageNum);
			post({ type: 'pdfjs:anchor-resolved', id: item.id, rects: rectsJson });
		} catch {
			// range inválido para esses spans — segue sem destaque
		}
	}
}

function clientRectsToNormalized(clientRects, wrap) {
	const wrapRect = wrap.getBoundingClientRect();
	const out = [];
	for (const r of clientRects) {
		if (r.width <= 0 || r.height <= 0) continue;
		out.push([
			(r.left - wrapRect.left) / wrapRect.width,
			(r.top - wrapRect.top) / wrapRect.height,
			r.width / wrapRect.width,
			r.height / wrapRect.height
		]);
	}
	return out;
}

// --- Sumário / capítulos ---

async function setupOutline(doc) {
	if (readonly && !outlineBtn) return;
	let outline;
	try {
		outline = await doc.getOutline();
	} catch {
		outline = null;
	}
	if (!outline || outline.length === 0) {
		outlineBtn?.remove();
		outlinePanel?.remove();
		return;
	}

	async function resolvePageIndex(dest) {
		try {
			const explicitDest = typeof dest === 'string' ? await doc.getDestination(dest) : dest;
			if (!explicitDest) return null;
			return await doc.getPageIndex(explicitDest[0]);
		} catch {
			return null;
		}
	}

	function buildList(items) {
		const ul = document.createElement('ul');
		for (const item of items) {
			const li = document.createElement('li');
			const link = document.createElement('a');
			link.href = '#';
			link.textContent = item.title;
			link.addEventListener('click', async (e) => {
				e.preventDefault();
				const index = await resolvePageIndex(item.dest);
				if (index !== null) scrollToPage(index + 1);
				outlinePanel.classList.add('hidden');
			});
			li.appendChild(link);
			if (item.items && item.items.length > 0) li.appendChild(buildList(item.items));
			ul.appendChild(li);
		}
		return ul;
	}

	outlinePanel.appendChild(buildList(outline));
	outlineBtn.classList.remove('hidden');
	outlineBtn.addEventListener('click', () => outlinePanel.classList.toggle('hidden'));
}

// --- Ciclo de vida principal ---

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

		for (let num = 1; num <= doc.numPages; num++) {
			const pdfPage = await doc.getPage(num);
			pageProxies.set(num, pdfPage);

			const wrap = document.createElement('div');
			wrap.className = 'page-wrap';
			wrap.dataset.page = String(num);

			const canvas = document.createElement('canvas');
			canvas.className = 'pdf-page';
			wrap.appendChild(canvas);

			const highlightLayer = document.createElement('div');
			highlightLayer.className = 'highlightLayer';
			wrap.appendChild(highlightLayer);

			const textLayerDiv = document.createElement('div');
			textLayerDiv.className = 'textLayer';
			wrap.appendChild(textLayerDiv);

			pagesContainer.appendChild(wrap);
			pageElements.set(num, wrap);

			await renderPage(num);
		}

		observePages();
		scrollToPage(initialPage, 'auto');
		await setupOutline(doc);
	} catch (err) {
		const message = err instanceof Error ? err.message : 'Falha ao carregar o PDF.';
		showStatus(message, true);
		post({ type: 'pdfjs:error', message });
	}
}

// --- Zoom / navegação de página ---

zoomOutBtn?.addEventListener('click', () => zoomBy(-1));
zoomInBtn?.addEventListener('click', () => zoomBy(1));
zoomFitBtn?.addEventListener('click', () => setZoom('fit-width'));

document.addEventListener(
	'wheel',
	(e) => {
		if (!e.ctrlKey) return;
		e.preventDefault();
		zoomBy(e.deltaY < 0 ? 1 : -1);
	},
	{ passive: false }
);

document.addEventListener('keydown', (e) => {
	if (!(e.ctrlKey || e.metaKey)) return;
	if (e.key === '+' || e.key === '=') {
		e.preventDefault();
		zoomBy(1);
	} else if (e.key === '-') {
		e.preventDefault();
		zoomBy(-1);
	}
});

pageInput?.addEventListener('keydown', (e) => {
	if (e.key !== 'Enter') return;
	const target = Math.max(1, Math.min(numPages || 1, parseInt(pageInput.value, 10) || currentPage));
	scrollToPage(target);
});

// --- Popover de comentário/nota ---

function openPopover(mode, data) {
	editingAnnotationId = mode === 'read' ? data.id : null;
	pendingComment = mode === 'anchored' ? data : null;
	noteTextarea.value = mode === 'read' ? data.note : '';
	notePopover.classList.remove('hidden');
	noteTextarea.focus();
}

function closePopover() {
	notePopover.classList.add('hidden');
	editingAnnotationId = null;
	pendingComment = null;
}

noteCancelBtn?.addEventListener('click', closePopover);

noteSaveBtn?.addEventListener('click', () => {
	const value = noteTextarea.value.trim();
	if (editingAnnotationId) {
		post({ type: 'pdfjs:update-annotation', id: editingAnnotationId, note: value });
	} else if (pendingComment) {
		post({ type: 'pdfjs:create-comment', page: currentPage, text: pendingComment.text, note: value, rects: pendingComment.rects });
	} else {
		post({ type: 'pdfjs:create-comment', page: currentPage, text: value, note: '', rects: '' });
	}
	closePopover();
});

// --- Barra de ferramentas de autoria (somente quando !readonly) ---

if (readonly) {
	commentBtn?.remove();
	selectionToolbar?.remove();
	notePopover?.remove();
} else {
	commentBtn.addEventListener('click', () => openPopover('page'));

	colorButtons.forEach((btn) => {
		btn.addEventListener('click', () => {
			selectedColor = btn.dataset.color;
			colorButtons.forEach((b) => b.classList.toggle('selected', b === btn));
		});
	});
	colorButtons.find((b) => b.dataset.color === selectedColor)?.classList.add('selected');

	document.addEventListener('mouseup', (event) => {
		if (selectionToolbar.contains(event.target)) return; // clique no próprio botão "Destacar"/"Comentar"
		const selection = window.getSelection();
		const text = selection ? selection.toString().trim() : '';
		if (!text || !selection.rangeCount) {
			selectionToolbar.classList.add('hidden');
			return;
		}
		const rect = selection.getRangeAt(0).getBoundingClientRect();
		selectionToolbar.style.top = `${window.scrollY + rect.top - 44}px`;
		selectionToolbar.style.left = `${window.scrollX + rect.left}px`;
		selectionToolbar.dataset.text = text;
		selectionToolbar.classList.remove('hidden');
	});

	document.addEventListener('mousedown', (event) => {
		if (!selectionToolbar.contains(event.target)) selectionToolbar.classList.add('hidden');
	});

	function selectionRectsForCurrentSelection() {
		const selection = window.getSelection();
		if (!selection || !selection.rangeCount) return { page: currentPage, rects: [] };
		const range = selection.getRangeAt(0);
		const wrap = (range.startContainer.nodeType === 1 ? range.startContainer : range.startContainer.parentElement)?.closest(
			'.page-wrap'
		);
		if (!wrap) return { page: currentPage, rects: [] };
		const page = Number(wrap.dataset.page) || currentPage;
		const rects = clientRectsToNormalized(range.getClientRects(), wrap);
		return { page, rects };
	}

	highlightBtn.addEventListener('click', () => {
		const text = selectionToolbar.dataset.text || '';
		const { page, rects } = selectionRectsForCurrentSelection();
		if (text) {
			post({ type: 'pdfjs:create-highlight', page, text, rects: JSON.stringify(rects), color: selectedColor });
		}
		selectionToolbar.classList.add('hidden');
		window.getSelection()?.removeAllRanges();
	});

	commentSelectionBtn.addEventListener('click', () => {
		const text = selectionToolbar.dataset.text || '';
		const { page, rects } = selectionRectsForCurrentSelection();
		selectionToolbar.classList.add('hidden');
		if (!text) return;
		currentPage = page;
		openPopover('anchored', { text, rects: JSON.stringify(rects) });
		window.getSelection()?.removeAllRanges();
	});
}

// --- Ponte host → iframe ---

window.addEventListener('message', (event) => {
	if (event.origin !== location.origin) return;
	const msg = event.data;
	if (!msg || typeof msg !== 'object') return;

	if (msg.type === 'pdfjs:set-inverted') {
		pagesContainer.classList.toggle('inverted', !!msg.value);
	} else if (msg.type === 'pdfjs:goto-page') {
		scrollToPage(Number(msg.page));
	} else if (msg.type === 'pdfjs:load-annotations') {
		annotationsByPage.clear();
		annotationsById.clear();
		for (const item of msg.items || []) {
			annotationsById.set(item.id, item);
			if (!annotationsByPage.has(item.page)) annotationsByPage.set(item.page, []);
			annotationsByPage.get(item.page).push(item);
		}
		for (const page of annotationsByPage.keys()) {
			renderHighlights(page);
			resolveUnanchoredHighlights(page);
		}
	} else if (msg.type === 'pdfjs:annotation-created') {
		const a = msg.annotation;
		annotationsById.set(a.id, a);
		if (!annotationsByPage.has(a.page)) annotationsByPage.set(a.page, []);
		annotationsByPage.get(a.page).push(a);
		renderHighlights(a.page);
	}
});

renderPDF();
