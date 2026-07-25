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
	collection_id: string;
	file_directory: string;
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
}

export interface Collection {
	id: string;
	name: string;
	description: string;
	is_default: boolean;
	created_at: string;
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
