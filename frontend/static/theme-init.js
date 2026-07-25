// Anti-flash de tema: roda antes da hidratação do Svelte, lendo o valor
// espelhado em localStorage (ver refatoracao/06-frontend.md, "Tema claro/
// escuro"). Externo (não inline) para respeitar a CSP script-src sem
// 'unsafe-inline' (ver refatoracao/08-seguranca.md).
(function () {
	try {
		var stored = localStorage.getItem('ui.theme') || 'system';
		var resolved =
			stored === 'system'
				? window.matchMedia('(prefers-color-scheme: dark)').matches
					? 'dark'
					: 'light'
				: stored;
		document.documentElement.setAttribute('data-theme', resolved);
	} catch (e) {}
})();
