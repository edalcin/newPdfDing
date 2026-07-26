import { apiJSON } from './api';
import type { PDF, PDFListPage, PDFSort } from './types';

// Rolagem infinita — sem paginação numerada em lugar nenhum da SPA (ver
// refatoracao/06-frontend.md, "Rolagem infinita"). O cursor é opaco; o
// servidor decide seu formato e a estabilidade sob inserção concorrente.

export class PDFListStore {
	items = $state<PDF[]>([]);
	cursor = $state<string | null>(null);
	loading = $state(false);
	done = $state(false);
	sort = $state<PDFSort>('newest');
	query = $state('');
	tag = $state('');

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
			const params = new URLSearchParams({ sort: this.sort, limit: '50' });
			if (this.cursor) params.set('cursor', this.cursor);
			if (this.query) params.set('q', this.query);
			if (this.tag) params.set('tag', this.tag);
			const page = await apiJSON<PDFListPage>(`/pdfs?${params.toString()}`);
			this.items = [...this.items, ...page.items];
			this.cursor = page.next_cursor;
			if (!page.next_cursor) this.done = true;
		} finally {
			this.loading = false;
		}
	}

	/** Replaces one item in place (e.g. after PATCH star/embed) without a
	 * full reload — keeps scroll position and avoids re-fetching the page. */
	replace(updated: PDF) {
		this.items = this.items.map((p) => (p.id === updated.id ? updated : p));
	}

	remove(id: string) {
		this.items = this.items.filter((p) => p.id !== id);
	}

	prepend(created: PDF) {
		this.items = [created, ...this.items];
	}
}
