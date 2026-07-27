// Camada de integração entre o SDK EmbedPDF e o contrato da API de
// anotações (internal/server/handlers_annotations.go). Nenhum componente
// vive aqui — só tradução de tipos e configuração.

import { PdfAnnotationSubtype } from '@embedpdf/snippet';
import type {
	AnnotationTransferItem,
	Locale,
	PdfAnnotationObject,
	PDFViewerConfig,
	Rect
} from '@embedpdf/snippet';
import type { Annotation, AnnotationKind } from './types';

// 3a. Locale pt-BR — o pacote só traz `en` e `es`; exportamos o dicionário
// inteiro (45 chaves de `commands`) explicitamente em português.
export const ptBR: Locale = {
	code: 'pt-BR',
	name: 'Português (Brasil)',
	translations: {
		commands: {
			zoom: {
				in: 'Aproximar',
				out: 'Afastar',
				fitWidth: 'Ajustar à largura',
				fitPage: 'Ajustar à página',
				automatic: 'Automático',
				level: 'Zoom ({level}%)',
				inArea: 'Aproximar área'
			},
			fullscreen: { enter: 'Tela cheia', exit: 'Sair da tela cheia' },
			rotate: { clockwise: 'Girar à direita', counterclockwise: 'Girar à esquerda' },
			menu: 'Menu',
			sidebar: 'Painel lateral',
			search: 'Buscar',
			comment: 'Comentário',
			download: 'Baixar',
			print: 'Imprimir',
			openFile: 'Abrir PDF',
			save: 'Salvar',
			settings: 'Configurações',
			view: 'Visualizar',
			annotate: 'Anotar',
			shapes: 'Formas',
			redact: 'Tarjar',
			fillAndSign: 'Preencher e assinar',
			form: 'Formulário',
			pan: 'Mover',
			pointer: 'Ponteiro',
			undo: 'Desfazer',
			redo: 'Refazer',
			copy: 'Copiar',
			screenshot: 'Captura de tela',
			nextPage: 'Próxima página',
			previousPage: 'Página anterior'
		}
	}
};

// 3b. Fábrica de config. wasmUrl/fontFallback/fonts apontam tudo para o
// próprio domínio — sem eles a CSP `default-src 'self'` do projeto derruba
// o viewer (ele buscaria pdfium.wasm e fontes do jsDelivr/Google Fonts).
export function viewerConfig(src: string, opts: { readonly?: boolean; author?: string } = {}): PDFViewerConfig {
	return {
		src,
		wasmUrl: '/embedpdf/pdfium.wasm',
		fontFallback: null,
		fonts: { ui: null, signature: null },
		i18n: { defaultLocale: 'pt-BR', fallbackLocale: 'en', locales: [ptBR] },
		disabledCategories: opts.readonly ? ['annotation', 'redaction', 'form'] : ['redaction', 'form'],
		annotations: { autoCommit: true, annotationAuthor: opts.author }
	};
}

// 3c. Mapeamento subtipo → kind. O CHECK da tabela pdf_annotations só admite
// 'comment'|'highlight', e as rotas /highlights e /comments dependem disso.
const MARKUP_SUBTYPES = new Set<PdfAnnotationSubtype>([
	PdfAnnotationSubtype.HIGHLIGHT,
	PdfAnnotationSubtype.UNDERLINE,
	PdfAnnotationSubtype.SQUIGGLY,
	PdfAnnotationSubtype.STRIKEOUT
]);

export function kindOf(a: PdfAnnotationObject): AnnotationKind {
	return MARKUP_SUBTYPES.has(a.type) ? 'highlight' : 'comment';
}

export interface AnnotationPayload {
	kind: AnnotationKind;
	page: number;
	text: string;
	note: string;
	color: string;
	data: string;
}

/** Projeta um AnnotationTransferItem do SDK (o mesmo formato que
 * importAnnotations consome) no corpo aceito por POST/PATCH
 * /api/.../annotations. `selectedText` é o texto selecionado no momento da
 * criação — só usado para subtipos de markup (highlight/underline/
 * squiggly/strikeout); reconstruir a partir de segmentRects não é confiável. */
export function toAnnotationPayload(item: AnnotationTransferItem, selectedText: string): AnnotationPayload {
	const a = item.annotation;
	const markup = MARKUP_SUBTYPES.has(a.type);
	// `color` está depreciado a favor de `strokeColor`, mas as duas formas
	// aparecem dependendo do subtipo — nenhuma existe na interface base.
	const c = a as PdfAnnotationObject & { strokeColor?: string; color?: string };
	return {
		kind: kindOf(a),
		page: a.pageIndex + 1,
		text: markup ? selectedText : '',
		note: a.contents ?? '',
		color: c.strokeColor ?? c.color ?? '#fde047',
		data: JSON.stringify(item)
	};
}

// 3d. Conversão de linha legada. Linhas com data === '' vieram do viewer
// antigo, com rects normalizado 0..1 e origem no canto superior esquerdo —
// a mesma orientação de Rect do EmbedPDF ({origin, size} em pontos da
// página, topo-esquerda). A conversão é multiplicação direta pelo tamanho
// da página.
export const LEGACY_HEX: Record<string, string> = {
	yellow: '#fde047',
	green: '#4ade80',
	blue: '#60a5fa',
	pink: '#f472b6'
};

function boundingRect(rects: Rect[]): Rect {
	const minX = Math.min(...rects.map((r) => r.origin.x));
	const minY = Math.min(...rects.map((r) => r.origin.y));
	const maxX = Math.max(...rects.map((r) => r.origin.x + r.size.width));
	const maxY = Math.max(...rects.map((r) => r.origin.y + r.size.height));
	return { origin: { x: minX, y: minY }, size: { width: maxX - minX, height: maxY - minY } };
}

/** Converte uma linha legada (rects preenchido, data vazio) num
 * AnnotationTransferItem pronto para importAnnotations. Retorna null para
 * uma linha sem geometria (`rects === ''`) — ela continua listada em
 * /highlights com seu texto, só não é desenhada sobre a página (era
 * recuperação de busca textual de um import único do banco Django; não
 * vale reimplementar sobre o novo motor). */
export function legacyToTransferItem(
	row: Annotation,
	pageSize: { width: number; height: number }
): AnnotationTransferItem | null {
	if (!row.rects) return null;
	const quads: Rect[] = (JSON.parse(row.rects) as number[][]).map(([x, y, w, h]) => ({
		origin: { x: x * pageSize.width, y: y * pageSize.height },
		size: { width: w * pageSize.width, height: h * pageSize.height }
	}));
	const bbox = boundingRect(quads);
	return {
		annotation: {
			type: PdfAnnotationSubtype.HIGHLIGHT,
			id: row.id,
			pageIndex: row.page - 1,
			rect: bbox,
			segmentRects: quads,
			opacity: 1,
			strokeColor: LEGACY_HEX[row.color] ?? LEGACY_HEX.yellow,
			contents: row.note
		}
	};
}
