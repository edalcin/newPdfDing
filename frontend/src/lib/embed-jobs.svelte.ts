// Polling store for the async embedding queue (ver refatoracao Fase F.4):
// while any pdf_id has a non-terminal job, polls GET /api/embed/jobs every
// 1500ms. Components register a callback to refresh their own PDF once its
// job disappears from the map (done) — the queue itself is a plain object
// keyed by pdf_id -> { state, error }.
import { apiJSON } from './api';

export interface EmbedJob {
	state: 'queued' | 'extracting' | 'embedding' | 'done' | 'failed';
	error?: string;
}

class EmbedJobsStore {
	jobs = $state<Record<string, EmbedJob>>({});

	private timer: number | null = null;
	private callbacks = new Map<string, () => void>();

	/** Registers pdfId so its callback fires once the job leaves the map
	 * (settled as done — a failed job stays visible until retried). */
	watch(pdfId: string, onSettled: () => void) {
		this.callbacks.set(pdfId, onSettled);
		this.start();
	}

	unwatch(pdfId: string) {
		this.callbacks.delete(pdfId);
	}

	start() {
		if (this.timer) return;
		this.poll();
		this.timer = setInterval(() => this.poll(), 1500);
	}

	stop() {
		if (this.timer) {
			clearInterval(this.timer);
			this.timer = null;
		}
	}

	private async poll() {
		try {
			const res = await apiJSON<{ jobs: Record<string, EmbedJob> }>('/embed/jobs');
			const previous = this.jobs;
			this.jobs = res.jobs;
			for (const [pdfId, callback] of this.callbacks) {
				if (previous[pdfId] && !res.jobs[pdfId]) {
					callback();
					this.callbacks.delete(pdfId);
				}
			}
			const hasNonTerminal = Object.keys(res.jobs).length > 0;
			if (!hasNonTerminal && this.callbacks.size === 0) this.stop();
		} catch {
			// best-effort — a failed poll just retries on the next tick
		}
	}
}

export const embedJobs = new EmbedJobsStore();
