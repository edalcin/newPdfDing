// Polling store for the async embedding queue (ver refatoracao Fase F.4).
//
// Quem exibe ícone de embedding se inscreve com onSettled(); a inscrição é
// que liga o polling. Isso vale para qualquer página e para qualquer job,
// inclusive um enfileirado antes do carregamento da página, por outra aba
// ou por outro dispositivo — não só para o job que o próprio componente
// acabou de clicar (era esse acoplamento que obrigava a recarregar a página
// para ver o processo terminar).
//
// Duas cadências: rápida enquanto existe job no mapa, lenta como batimento
// quando o mapa está vazio (é o batimento que descobre um job de fora).
// Aba oculta não gera requisição; ao voltar a ficar visível, poleia na hora.
import { apiJSON } from './api';

export interface EmbedJob {
	state: 'queued' | 'extracting' | 'embedding' | 'done' | 'failed';
	error?: string;
}

const ACTIVE_MS = 1500;
const IDLE_MS = 10000;

class EmbedJobsStore {
	jobs = $state<Record<string, EmbedJob>>({});

	private timer: number | null = null;
	private listeners = new Set<(pdfId: string) => void>();
	private visibilityBound = false;

	/** Registra um ouvinte chamado com o pdf_id de cada job que sai do mapa
	 * (concluído — um job falho permanece visível até ser reenfileirado).
	 * Retorna a função de cancelamento, para chamar no onDestroy da página. */
	onSettled(listener: (pdfId: string) => void): () => void {
		this.listeners.add(listener);
		this.bindVisibility();
		this.schedule(0);
		return () => {
			this.listeners.delete(listener);
			if (this.listeners.size === 0) this.stop();
		};
	}

	stop() {
		if (this.timer) {
			clearTimeout(this.timer);
			this.timer = null;
		}
	}

	private bindVisibility() {
		if (this.visibilityBound || typeof document === 'undefined') return;
		this.visibilityBound = true;
		document.addEventListener('visibilitychange', () => {
			if (!document.hidden && this.listeners.size > 0) this.schedule(0);
		});
	}

	private schedule(delay: number) {
		this.stop();
		this.timer = window.setTimeout(() => this.tick(), delay);
	}

	private async tick() {
		if (typeof document === 'undefined' || !document.hidden) await this.poll();
		if (this.listeners.size === 0) {
			this.timer = null;
			return;
		}
		this.schedule(this.hasRunningJob() ? ACTIVE_MS : IDLE_MS);
	}

	/** Cadência rápida só enquanto há trabalho em curso. Um job 'failed'
	 * fica no mapa do servidor até ser reenfileirado, e 'done' fica 60s —
	 * nenhum dos dois justifica poleio de 1,5s indefinidamente. */
	private hasRunningJob(): boolean {
		return Object.values(this.jobs).some(
			(job) => job.state === 'queued' || job.state === 'extracting' || job.state === 'embedding'
		);
	}

	/** Uma varredura imediata, fora da cadência — usada logo após enfileirar,
	 * para o rótulo sair de "Embedar" sem esperar o próximo tique. */
	async poll() {
		try {
			const res = await apiJSON<{ jobs: Record<string, EmbedJob> }>('/embed/jobs');
			const previous = this.jobs;
			this.jobs = res.jobs;
			for (const pdfId of Object.keys(previous)) {
				if (!res.jobs[pdfId]) {
					for (const listener of this.listeners) listener(pdfId);
				}
			}
		} catch {
			// best-effort — o mapa anterior permanece até o próximo tique
		}
	}
}

export const embedJobs = new EmbedJobsStore();
