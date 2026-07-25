import tailwindcss from '@tailwindcss/vite';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},

			// SPA pura: fallback: 'index.html' — o roteamento de tela é feito no
			// cliente, o binário Go só serve arquivos estáticos (ver
			// refatoracao/06-frontend.md, "Saída de build e integração com go:embed").
			adapter: adapter({
				fallback: 'index.html'
			}),

			// SvelteKit calcula o hash sha256 do seu script de bootstrap inline
			// (necessário mesmo com adapter-static) e injeta um <meta> CSP no
			// index.html gerado. O servidor Go lê esse hash na inicialização e
			// mescla no header CSP real — nonce não funciona em páginas
			// estáticas pré-renderizadas (ver refatoracao/08-seguranca.md).
			csp: {
				mode: 'hash',
				directives: {
					'script-src': ['self', 'wasm-unsafe-eval']
				}
			}
		})
	]
});
