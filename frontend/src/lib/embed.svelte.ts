// Estado de sessão do botão de embedding (ver refatoracao/06-frontend.md,
// "Botão de embedding — máquina de estados"): um 412 (GEMINI_API_KEY
// ausente) desabilita o botão em TODOS os documentos da sessão, não só no
// que foi clicado.

class EmbedSessionState {
	disabledGlobally = $state(false);
}

export const embedSession = new EmbedSessionState();
