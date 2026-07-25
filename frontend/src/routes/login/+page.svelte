<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { goto } from '$app/navigation';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';

	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		loading = true;
		try {
			await auth.login(password);
			goto('/');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Falha ao entrar';
		} finally {
			loading = false;
		}
	}
</script>

<div class="flex min-h-screen items-center justify-center px-4">
	<form onsubmit={handleSubmit} class="w-full max-w-sm space-y-4 rounded-lg border border-border p-6">
		<h1 class="text-lg font-semibold">newPdfDing</h1>
		<div class="space-y-1">
			<label for="password" class="text-sm font-medium">Senha</label>
			<Input id="password" type="password" bind:value={password} required autofocus />
		</div>
		{#if error}
			<p class="text-sm text-destructive">{error}</p>
		{/if}
		<Button type="submit" class="w-full" disabled={loading}>
			{loading ? 'Entrando…' : 'Entrar'}
		</Button>
	</form>
</div>
