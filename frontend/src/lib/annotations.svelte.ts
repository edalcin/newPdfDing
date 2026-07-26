import { apiJSON } from './api';
import type { Annotation, AnnotationKind, AnnotationListPage } from './types';

// Rolagem infinita para /highlights e /comments, e para a listagem por PDF
// na página de detalhes — mesmo padrão de cursor de PDFListStore (ver
// refatoracao/06-frontend.md, "Rolagem infinita").

export class AnnotationListStore {
	items = $state<Annotation[]>([]);
	cursor = $state<string | null>(null);
	loading = $state(false);
	done = $state(false);
	kind = $state<AnnotationKind | ''>('');
	pdfId = $state('');

	async reset() {
		this.items = [];
		this.cursor = null;
		this.done = false;
		await this.loadMore();
	}

	async loadMore() {
		if (this.loading || this.done) return;
		this.loading = true;
		try {
			const params = new URLSearchParams();
			if (this.kind) params.set('kind', this.kind);
			if (this.pdfId) params.set('pdf_id', this.pdfId);
			if (this.cursor) params.set('cursor', this.cursor);
			const page = await apiJSON<AnnotationListPage>(`/annotations?${params.toString()}`);
			this.items = [...this.items, ...page.items];
			this.cursor = page.next_cursor;
			if (!page.next_cursor) this.done = true;
		} finally {
			this.loading = false;
		}
	}

	remove(id: string) {
		this.items = this.items.filter((a) => a.id !== id);
	}

	prepend(created: Annotation) {
		this.items = [created, ...this.items];
	}
}

export async function createAnnotation(
	pdfId: string,
	kind: AnnotationKind,
	page: number,
	text: string
): Promise<Annotation> {
	return apiJSON<Annotation>(`/pdfs/${pdfId}/annotations`, {
		method: 'POST',
		body: { kind, page, text }
	});
}

export async function deleteAnnotation(id: string): Promise<void> {
	await apiJSON(`/annotations/${id}`, { method: 'DELETE' });
}

/** Triggers a browser download of the export via a hidden link — the
 * endpoint sets Content-Disposition, so a plain navigation is enough. */
export function exportAnnotationsUrl(kind: AnnotationKind | '', pdfId: string, format: 'json' | 'yaml'): string {
	const params = new URLSearchParams({ format });
	if (kind) params.set('kind', kind);
	if (pdfId) params.set('pdf_id', pdfId);
	return `/api/annotations/export?${params.toString()}`;
}
