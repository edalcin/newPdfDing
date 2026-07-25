<script lang="ts">
	let { onIntersect, disabled = false }: { onIntersect: () => void; disabled?: boolean } = $props();
	let el = $state<HTMLDivElement>();

	$effect(() => {
		if (disabled || !el) return;
		const target = el;
		const observer = new IntersectionObserver((entries) => {
			if (entries[0].isIntersecting) onIntersect();
		});
		observer.observe(target);
		return () => observer.disconnect();
	});
</script>

<div bind:this={el} class="h-1" aria-hidden="true"></div>
