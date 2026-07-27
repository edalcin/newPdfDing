<script lang="ts">
	// Componente compartilhado entre /viewer/[id] (autoria completa) e
	// /s/[share] (somente leitura) — monta o SDK EmbedPDF, aplica o filtro
	// de "inverter cores" só sobre o <canvas> das páginas (a barra de
	// ferramentas do SDK fica de fora, ao contrário do viewer.mjs antigo
	// cujo #pagesContainer já isolava isso), e restaura a página inicial
	// assim que o layout do scroll estiver pronto.
	import { PDFViewer, type PluginRegistry, ScrollPlugin } from '@embedpdf/svelte-pdf-viewer';
	import { viewerConfig } from '$lib/embedpdf';

	let {
		src,
		readonly = false,
		inverted = false,
		initialPage = 1,
		author,
		onready,
		onpagechange
	}: {
		src: string;
		readonly?: boolean;
		inverted?: boolean;
		initialPage?: number;
		author?: string;
		onready?: (registry: PluginRegistry) => void;
		onpagechange?: (page: number) => void;
	} = $props();

	async function handleReady(registry: PluginRegistry) {
		await registry.pluginsReady();
		const scroll = registry.getPlugin<ScrollPlugin>('scroll')!.provides();
		scroll.onLayoutReady(() => {
			if (initialPage > 1) scroll.scrollToPage({ pageNumber: initialPage });
		});
		scroll.onPageChange((e) => onpagechange?.(e.pageNumber));
		onready?.(registry);
	}
</script>

<div class="h-full w-full" class:inverted>
	<PDFViewer config={viewerConfig(src, { readonly, author })} class="h-full w-full" onready={handleReady} />
</div>

<style>
	.inverted :global(canvas) {
		filter: invert(1) hue-rotate(180deg);
	}
</style>
