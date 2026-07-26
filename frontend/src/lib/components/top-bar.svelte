<script lang="ts">
	import { theme, type ThemeSetting } from '$lib/theme.svelte';
	import { auth } from '$lib/auth.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
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

	const NAV_LINKS: { href: string; icon: string; label: string }[] = [
		{ href: '/', icon: 'bx-library', label: 'Biblioteca' },
		{ href: '/highlights', icon: 'bx-highlight', label: 'Destaques' },
		{ href: '/comments', icon: 'bx-comment-detail', label: 'Comentários' },
		{ href: '/tags', icon: 'bx-purchase-tag', label: 'Tags' },
		{ href: '/settings', icon: 'bx-cog', label: 'Configurações' },
		{ href: '/admin', icon: 'bx-shield-quarter', label: 'Administração' }
	];

	async function cycleTheme() {
		await theme.set(nextTheme[theme.value]);
	}

	async function handleLogout() {
		await auth.logout();
		goto('/login');
	}
</script>

<header class="flex h-14 items-center justify-between gap-2 border-b border-border px-4">
	<a href="/" class="shrink-0 text-sm font-semibold">newPdfDing</a>
	<nav class="flex flex-1 items-center gap-1 overflow-x-auto">
		{#each NAV_LINKS as link (link.href)}
			<a
				href={link.href}
				class="flex shrink-0 items-center gap-1.5 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-accent hover:text-accent-foreground {page
					.url.pathname === link.href
					? 'bg-accent text-accent-foreground'
					: 'text-muted-foreground'}"
			>
				<i class={`bx ${link.icon}`}></i>
				<span class="hidden md:inline">{link.label}</span>
			</a>
		{/each}
	</nav>
	<div class="flex shrink-0 items-center gap-2">
		<Button variant="ghost" size="icon" onclick={cycleTheme} aria-label="Alternar tema ({theme.value})">
			<i class={`bx ${themeIcon[theme.value]} text-lg`}></i>
		</Button>
		<Button variant="ghost" size="icon" onclick={handleLogout} aria-label="Sair">
			<i class="bx bx-log-out text-lg"></i>
		</Button>
	</div>
</header>
