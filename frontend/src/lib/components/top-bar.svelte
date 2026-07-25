<script lang="ts">
	import { theme, type ThemeSetting } from '$lib/theme.svelte';
	import { auth } from '$lib/auth.svelte';
	import { goto } from '$app/navigation';
	import { Button } from '$lib/components/ui/button';

	const themeIcon: Record<ThemeSetting, string> = {
		system: 'bx-desktop',
		light: 'bx-sun',
		dark: 'bx-moon'
	};
	const nextTheme: Record<ThemeSetting, ThemeSetting> = {
		system: 'light',
		light: 'dark',
		dark: 'system'
	};

	async function cycleTheme() {
		await theme.set(nextTheme[theme.value]);
	}

	async function handleLogout() {
		await auth.logout();
		goto('/login');
	}
</script>

<header class="flex h-14 items-center justify-between border-b border-border px-4">
	<a href="/" class="text-sm font-semibold">newPdfDing</a>
	<div class="flex items-center gap-2">
		<Button variant="ghost" size="icon" onclick={cycleTheme} aria-label="Alternar tema ({theme.value})">
			<i class={`bx ${themeIcon[theme.value]} text-lg`}></i>
		</Button>
		<Button variant="ghost" size="icon" onclick={handleLogout} aria-label="Sair">
			<i class="bx bx-log-out text-lg"></i>
		</Button>
	</div>
</header>
