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
import { enUS, esES } from '@embedpdf/plugin-i18n';
import type { Annotation, AnnotationKind } from './types';

// 3a. Locale pt-BR. IMPORTANTE: os dicionários `enUS`/`esES` embutidos no
// pacote `@embedpdf/plugin-i18n` só cobrem o namespace `commands.*` (~26
// chaves) — e esse namespace nunca é referenciado pelo schema de UI
// pré-construído do snippet (`embedpdf-*.js`), que usa ~250 chaves sob
// outros ~35 namespaces (`document.*`, `mode.*`, `zoom.*`, `annotation.*`,
// `panel.*`, etc — confirmado por grep no bundle publicado). Sem tradução
// própria para essas chaves reais, a UI mostra a chave literal ("mode.view",
// "document.open") em qualquer idioma, inclusive inglês, porque o pacote
// não embute (nem busca por rede) nenhum texto padrão para elas. Este
// dicionário cobre o namespace real por completo.
export const ptBR: Locale = {
	code: 'pt-BR',
	name: 'Português (Brasil)',
	translations: {
		annotation: {
			addLink: 'Adicionar link',
			arrow: 'Seta',
			blendMode: 'Modo de mesclagem',
			borderStyle: 'Estilo da borda',
			callout: 'Chamada',
			caret: 'Acento',
			circle: 'Círculo',
			comment: 'Comentário',
			defaults: 'Padrões de {type}',
			deleteAllSelected: 'Excluir todos selecionados',
			deleteSelected: 'Excluir selecionado',
			fillColor: 'Cor de preenchimento',
			fontColor: 'Cor da fonte',
			fontFamily: 'Família da fonte',
			fontSize: 'Tamanho da fonte',
			freeText: 'Texto livre',
			gotoLink: 'Ir para o link',
			group: 'Agrupar',
			highlight: 'Destaque',
			ink: 'Tinta',
			inkHighlighter: 'Marca-texto',
			insertText: 'Inserir texto',
			line: 'Linha',
			lineEnd: 'Fim da linha',
			lineEndings: 'Terminações de linha',
			lineStart: 'Início da linha',
			link: 'Link',
			moreTools: 'Mais ferramentas',
			multiSelect: '{count} selecionados',
			opacity: 'Opacidade',
			overlayText: 'Texto sobreposto',
			overlayTextPlaceholder: 'Digite o texto sobreposto',
			polygon: 'Polígono',
			polyline: 'Polilinha',
			rectangle: 'Retângulo',
			redact: 'Tarjar',
			removeLink: 'Remover link',
			replaceText: 'Substituir texto',
			rotation: 'Rotação',
			selectAnnotation: 'Selecionar anotação',
			square: 'Quadrado',
			squiggly: 'Ondulado',
			stamp: 'Carimbo',
			strikeout: 'Tachado',
			strokeColor: 'Cor do traço',
			strokeWidth: 'Espessura do traço',
			style: 'Estilo',
			styles: 'Estilos de {type}',
			text: 'Texto',
			textAlign: 'Alinhamento do texto',
			underline: 'Sublinhado',
			ungroup: 'Desagrupar',
			verticalAlign: 'Alinhamento vertical',
			widgetEdit: 'Editar campo'
		},
		blendMode: {
			color: 'Cor',
			colorBurn: 'Escurecer cor',
			colorDodge: 'Subexposição de cor',
			darken: 'Escurecer',
			difference: 'Diferença',
			exclusion: 'Exclusão',
			hardLight: 'Luz forte',
			hue: 'Matiz',
			lighten: 'Clarear',
			luminosity: 'Luminosidade',
			multiply: 'Multiplicar',
			normal: 'Normal',
			overlay: 'Sobrepor',
			saturation: 'Saturação',
			screen: 'Tela',
			softLight: 'Luz suave'
		},
		capture: {
			cancel: 'Cancelar',
			download: 'Baixar',
			dragTip: 'Arraste para selecionar uma área',
			screenshot: 'Captura de tela',
			title: 'Capturar tela'
		},
		comments: {
			addComment: 'Adicionar comentário',
			addReply: 'Adicionar resposta',
			cancel: 'Cancelar',
			closeAllAnnotations: 'Fechar todas as anotações',
			commentCount: '{count} comentário',
			commentCountPlural: '{count} comentários',
			delete: 'Excluir',
			edit: 'Editar',
			emptyState: 'Nenhum comentário ainda',
			page: 'Página {page}',
			save: 'Salvar',
			showAllAnnotations: 'Mostrar todas as anotações',
			showLess: 'Mostrar menos',
			showMore: 'Mostrar mais'
		},
		common: {
			back: 'Voltar',
			cancel: 'Cancelar',
			close: 'Fechar'
		},
		document: {
			close: 'Fechar documento',
			export: 'Exportar',
			fullscreen: 'Tela cheia',
			loading: 'Carregando documento…',
			menu: 'Menu',
			open: 'Abrir',
			pdf: 'PDF',
			print: 'Imprimir',
			protect: 'Proteger'
		},
		documentError: {
			close: 'Fechar',
			errorCode: 'Código do erro: {code}',
			title: 'Erro ao carregar o documento',
			unknown: 'Ocorreu um erro desconhecido'
		},
		emptyState: {
			description: 'Arraste um arquivo PDF aqui ou clique para selecionar',
			descriptionMulti: 'Arraste arquivos PDF aqui ou clique para selecionar',
			openButton: 'Abrir arquivo',
			supportedFormats: 'Formatos suportados: PDF',
			title: 'Nenhum documento aberto'
		},
		form: {
			addOption: 'Adicionar opção',
			checkbox: 'Caixa de seleção',
			comb: 'Caixas de combinação',
			combobox: 'Caixa combinada',
			defaultValue: 'Valor padrão',
			fieldName: 'Nome do campo',
			listbox: 'Lista de seleção',
			maxLength: 'Tamanho máximo',
			multiSelect: 'Seleção múltipla',
			multiline: 'Múltiplas linhas',
			options: 'Opções',
			properties: 'Propriedades',
			radio: 'Opção',
			radiobutton: 'Botão de opção',
			readOnly: 'Somente leitura',
			removeOption: 'Remover opção',
			required: 'Obrigatório',
			select: 'Lista suspensa',
			textfield: 'Campo de texto',
			toggleFillMode: 'Alternar modo de preenchimento'
		},
		history: {
			redo: 'Refazer',
			undo: 'Desfazer'
		},
		insert: {
			image: 'Imagem',
			rubberStamp: 'Carimbo'
		},
		link: {
			enterPage: 'Digite o número da página',
			enterUrl: 'Digite a URL',
			link: 'Link',
			page: 'Página',
			pageRange: 'Página 1–{totalPages}',
			title: 'Link',
			url: 'URL'
		},
		menu: {
			moreOptions: 'Mais opções',
			viewControls: 'Controles de visualização',
			zoomControls: 'Controles de zoom'
		},
		mode: {
			annotate: 'Anotar',
			form: 'Formulário',
			insert: 'Inserir',
			redact: 'Tarjar',
			shapes: 'Formas',
			view: 'Visualizar'
		},
		outline: {
			loading: 'Carregando sumário…',
			noBookmarks: 'Nenhum marcador',
			noOutline: 'Nenhum sumário disponível',
			title: 'Sumário'
		},
		page: {
			horizontal: 'Horizontal',
			next: 'Próxima página',
			previous: 'Página anterior',
			rotation: 'Rotação',
			scrollLayout: 'Layout de rolagem',
			settings: 'Configurações de página',
			single: 'Página única',
			spreadMode: 'Modo de exibição',
			twoEven: 'Duas páginas (par)',
			twoOdd: 'Duas páginas (ímpar)',
			vertical: 'Vertical'
		},
		pan: {
			toggle: 'Mover'
		},
		panel: {
			annotationStyle: 'Estilo da anotação',
			comment: 'Comentários',
			outline: 'Sumário',
			redaction: 'Tarjamento',
			search: 'Buscar',
			sidebar: 'Painel lateral',
			thumbnails: 'Miniaturas'
		},
		passwordPrompt: {
			cancel: 'Cancelar',
			incorrect: 'Senha incorreta',
			incorrectWarning: 'A senha informada está incorreta. Tente novamente.',
			label: 'Senha',
			open: 'Abrir',
			opening: 'Abrindo…',
			placeholder: 'Digite a senha',
			required: 'Este documento requer uma senha',
			title: 'Documento protegido por senha'
		},
		pointer: {
			toggle: 'Ponteiro'
		},
		print: {
			all: 'Todas as páginas',
			annotation: 'Incluir anotações',
			cancel: 'Cancelar',
			current: 'Página {currentPage} de {totalPages}',
			loading: 'Preparando impressão…',
			pages: 'Páginas',
			print: 'Imprimir',
			printing: 'Imprimindo…',
			specify: 'Especificar páginas',
			specifyEG: 'ex.: 1-3, 5, 8-10',
			title: 'Imprimir'
		},
		protect: {
			apply: 'Aplicar',
			applyFailed: 'Falha ao aplicar a proteção',
			applying: 'Aplicando…',
			bothPasswordsNote: 'Defina uma senha de usuário, uma senha de proprietário, ou ambas',
			cancel: 'Cancelar',
			noProtectionSelected: 'Nenhuma proteção selecionada',
			passwordMismatch: 'As senhas não coincidem',
			removeFailed: 'Falha ao remover a proteção',
			title: 'Proteger documento'
		},
		react: {
			element: 'Elemento'
		},
		redaction: {
			apply: 'Aplicar tarja',
			applyAll: 'Aplicar todas',
			area: 'Área',
			clearAll: 'Limpar tudo',
			commitSelected: 'Confirmar selecionado',
			deleteSelected: 'Excluir selecionado',
			emptyState: 'Nenhuma tarja ainda',
			redact: 'Tarjar',
			text: 'Texto'
		},
		rotate: {
			clockwise: 'Girar à direita',
			counterClockwise: 'Girar à esquerda'
		},
		search: {
			caseSensitive: 'Diferenciar maiúsculas/minúsculas',
			page: 'Página {page}',
			placeholder: 'Buscar no documento',
			resultsFound: '{count} resultados encontrados',
			wholeWord: 'Palavra inteira'
		},
		selection: {
			copy: 'Copiar',
			copyToClipboard: 'Copiar para a área de transferência'
		},
		signature: {
			createNew: 'Criar nova assinatura',
			createNewWithInitials: 'Criar nova assinatura com iniciais',
			emptyState: 'Nenhuma assinatura ainda',
			ink: 'Desenhar',
			placeInitials: 'Colocar iniciais',
			placeSignature: 'Colocar assinatura',
			remove: 'Remover',
			stamp: 'Carimbo',
			title: 'Assinatura'
		},
		stamp: {
			allStamps: 'Todos os carimbos',
			createFromGroup: 'Criar a partir do grupo',
			createFromSelected: 'Criar a partir da seleção',
			emptyState: 'Nenhum carimbo ainda',
			rubberStamp: 'Carimbo',
			title: 'Carimbos'
		},
		tabs: {
			overflowMenu: 'Mais abas'
		},
		zoom: {
			dragTip: 'Arraste para aproximar a área',
			fitPage: 'Ajustar à página',
			fitWidth: 'Ajustar à largura',
			in: 'Aproximar',
			level: 'Zoom ({level}%)',
			marquee: 'Zoom em área',
			menu: 'Zoom',
			out: 'Afastar'
		}
	}
};

// 3b. Fábrica de config. wasmUrl/fontFallback/fonts apontam tudo para o
// próprio domínio — sem eles a CSP `default-src 'self'` do projeto derruba
// o viewer (ele buscaria pdfium.wasm e fontes do jsDelivr/Google Fonts).
// worker: true (padrão do SDK) roda o motor PDFium num Web Worker — a CSP
// do projeto declara `worker-src 'self' blob:` (ver internal/security/
// headers.go) exatamente para permitir o worker que o EmbedPDF cria a
// partir de uma blob: URL. Antes desta diretiva existir, `worker: false`
// forçava a rasterização para a thread principal e travava a aba em PDFs
// grandes — ver refatoracao/11-desempenho-viewer.md (causa C1) para o
// diagnóstico completo. O plugin de carimbo busca sua biblioteca padrão
// em `manifests[0].url` (cdn.jsdelivr.net) por config própria — não é
// `defaultLibrary` (esse é só o rótulo da pasta "Custom Stamps" local).
// `manifests: []` remove essa fonte remota; o botão de carimbo continua
// funcionando para upload de imagem própria (biblioteca "Custom Stamps").
// tiling/render: tileSize maior reduz o número de tiles por página (cada
// tile reexecuta a display list inteira via FPDF_RenderPageBitmapWithMatrix
// — não há renderização parcial de content stream no PDFium), e WebP a
// qualidade 0.8 é mais barato de codificar que o PNG padrão do SDK para
// tiles grandes (11-desempenho-viewer.md, causa C2 e Fase 3). Valores não
// medidos em produção ainda — ajustar se o perfil de desempenho pedir.
// wasmUrl precisa ser uma URL ABSOLUTA, não um caminho relativo: com
// worker:true, o PDFium roda num Web Worker instanciado a partir de uma
// blob: URL (ver comentário acima), e blob: não serve de base para
// resolução de URL relativa — new URL('/x', 'blob:...') lança
// "Invalid URL" dentro do worker (confirmado em produção: nenhuma
// requisição a pdfium.wasm sequer parte, sem erro de CSP, sem erro de
// rede — só falha de parse silenciosa, dentro do worker, invisível ao
// console da página principal). Com worker:false (config antiga) isso
// nunca apareceu porque o motor rodava na thread principal, cuja
// location já é a origem real da página. window.location.origin só
// existe no browser — esta função só é chamada em componentes .svelte,
// nunca durante SSR/prerender do adapter-static.
export function viewerConfig(src: string, opts: { readonly?: boolean; author?: string } = {}): PDFViewerConfig {
	return {
		src,
		wasmUrl: `${window.location.origin}/embedpdf/pdfium.wasm`,
		fontFallback: null,
		fonts: { ui: null, signature: null },
		i18n: { defaultLocale: 'pt-BR', fallbackLocale: 'en', locales: [ptBR, enUS, esES] },
		disabledCategories: opts.readonly ? ['annotation', 'redaction', 'form'] : ['redaction', 'form'],
		annotations: { autoCommit: true, annotationAuthor: opts.author },
		stamp: { manifests: [] },
		tiling: { tileSize: 1536 },
		render: { defaultImageType: 'image/webp', defaultImageQuality: 0.8 }
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
