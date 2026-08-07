<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import * as Tooltip from '$lib/components/shad/tooltip';

	let { caption, children }: { caption: string; children: Snippet } = $props();
	let disabled = $state(true);
	onMount(() => {
		setTimeout(() => {
			disabled = false;
		}, 10);
	});
</script>

<Tooltip.Provider>
	<Tooltip.Root {disabled}>
		<Tooltip.Trigger>
			{@render children()}
		</Tooltip.Trigger>
		<Tooltip.Content class="max-w-50 text-center">
			{caption}
		</Tooltip.Content>
	</Tooltip.Root>
</Tooltip.Provider>
