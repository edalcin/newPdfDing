import { apiJSON } from './api';

// Tema claro/escuro/sistema — persistido em settings['ui.theme'] no
// servidor e espelhado em localStorage para aplicar antes da hidratação
// (script em app.html), evitando flash de tema incorreto (ver
// refatoracao/06-frontend.md, "Tema claro/escuro").

export type ThemeSetting = 'system' | 'light' | 'dark';

const STORAGE_KEY = 'ui.theme';

function initial(): ThemeSetting {
	if (typeof localStorage === 'undefined') return 'system';
	const stored = localStorage.getItem(STORAGE_KEY);
	return stored === 'light' || stored === 'dark' || stored === 'system' ? stored : 'system';
}

class ThemeStore {
	value = $state<ThemeSetting>(initial());

	resolved(): 'light' | 'dark' {
		if (this.value !== 'system') return this.value;
		if (typeof window === 'undefined') return 'light';
		return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
	}

	apply() {
		if (typeof document === 'undefined') return;
		document.documentElement.setAttribute('data-theme', this.resolved());
	}

	/** Loads the server's authoritative value once a session exists. */
	async loadFromServer() {
		try {
			const settings = await apiJSON<Record<string, string>>('/settings');
			const serverValue = settings['ui.theme'];
			if (serverValue === 'light' || serverValue === 'dark' || serverValue === 'system') {
				this.value = serverValue;
				localStorage.setItem(STORAGE_KEY, serverValue);
				this.apply();
			}
		} catch {
			// Not authenticated yet, or request failed — localStorage mirror stands.
		}
	}

	async set(next: ThemeSetting) {
		this.value = next;
		localStorage.setItem(STORAGE_KEY, next);
		this.apply();
		try {
			await apiJSON('/settings', { method: 'PATCH', body: { [STORAGE_KEY]: next } });
		} catch {
			// Server persistence is best-effort here — localStorage already
			// holds the value for the anti-flash script on next load.
		}
	}
}

export const theme = new ThemeStore();
