// Tipos espelhando exatamente o contrato de refatoracao/05-api.md,
// "Representação de PDF" e demais entidades.

export interface Tag {
	id: string;
	name: string;
}

export interface TagWithCount extends Tag {
	count: number;
}

export type EmbeddingStatus = 'none' | 'current' | 'stale';

export interface PDF {
	id: string;
	name: string;
	description: string;
	notes: string;
	notes_html: string;
	sha256: string;
	size_bytes: number;
	num_pages: number;
	current_page: number;
	views: number;
	revision: number;
	starred: boolean;
	archived: boolean;
	created_at: string;
	last_viewed_at: string | null;
	tags: Tag[];
	embedding_status: EmbeddingStatus;
	has_text: boolean;
}

export type PDFSort =
	| 'newest'
	| 'oldest'
	| 'name_asc'
	| 'name_desc'
	| 'most_viewed'
	| 'least_viewed'
	| 'recently_viewed';

export type Layout = 'compact' | 'list' | 'grid' | 'minimal';

export interface PDFListPage {
	items: PDF[];
	next_cursor: string | null;
}

export type AnnotationKind = 'comment' | 'highlight';

export interface Annotation {
	id: string;
	pdf_id: string;
	kind: AnnotationKind;
	page: number;
	text: string;
	note: string;
	color: string;
	rects: string;
	data: string;
	created_at: string;
}

export interface AnnotationListPage {
	items: Annotation[];
	next_cursor: string | null;
}

export interface Share {
	id: string;
	pdf_id: string;
	pdf_name: string;
	views: number;
	created_at: string;
}

export interface AdminInfo {
	pdfs_count: number;
	tags_count: number;
	files_bytes: number;
	embedding_status_counts: Record<EmbeddingStatus, number>;
}

// Chaves fechadas de settings (ver refatoracao/02-modelo-de-dados.md,
// "Chaves de configuração").
export interface Settings {
	'ui.theme': 'system' | 'light' | 'dark';
	'ui.layout': Layout;
	'ui.per_page': string;
	'ui.tags_open': '0' | '1';
	'ui.show_progress_bars': '0' | '1';
	'pdf.sorting': PDFSort;
	'annotation.sorting': string;
	'viewer.inverted': '0' | '1';
	'viewer.keep_awake': '0' | '1';
}
