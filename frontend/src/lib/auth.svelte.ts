import { apiJSON, apiRequest } from './api';

// Sessão SPA — o cookie de sessão é HttpOnly (ver refatoracao/08-seguranca.md),
// então o frontend nunca o lê diretamente; ele só sabe se está autenticado
// perguntando ao servidor.

class AuthStore {
	authenticated = $state<boolean | null>(null); // null = ainda não verificado

	async check(): Promise<boolean> {
		try {
			const res = await apiJSON<{ authenticated: boolean }>('/auth/session');
			this.authenticated = res.authenticated;
		} catch {
			this.authenticated = false;
		}
		return this.authenticated;
	}

	async login(password: string): Promise<void> {
		await apiJSON('/auth/login', { method: 'POST', body: { password } });
		this.authenticated = true;
	}

	async logout(): Promise<void> {
		await apiRequest('/auth/logout', { method: 'POST' });
		this.authenticated = false;
	}
}

export const auth = new AuthStore();
