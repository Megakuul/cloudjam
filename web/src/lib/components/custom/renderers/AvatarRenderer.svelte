<script lang="ts">
	import * as Avatar from '$lib/components/shad/avatar/index';
	import { toShortInitials } from '$lib';
	import WrappedTooltip from '$lib/components/custom/WrappedTooltip.svelte';

	let {
		image,
		name,
		height,
		width,
		useDiceBear = false
	}: {
		image: string | null;
		name: string;
		height?: string;
		width?: string;
		useDiceBear?: boolean;
	} = $props();
</script>

<WrappedTooltip caption={name}>
	<Avatar.Root class="{height ?? 'h-12'} {width ?? 'w-12'}">
		<Avatar.Image alt={name} src={image} />
		<Avatar.Fallback>
			{#if useDiceBear}
				<img alt={name} id="fallback" src={`https://api.dicebear.com/9.x/notionists/svg?seed=${name}`} />
			{:else}
				{toShortInitials(name)}
			{/if}
		</Avatar.Fallback>
	</Avatar.Root>
</WrappedTooltip>
