<script lang="ts">
	import * as Avatar from '$lib/components/shad/avatar/index';
	import { toShortInitials } from '$lib';
	import WrappedTooltip from '$lib/components/custom/WrappedTooltip.svelte';
	import { toSvg } from 'jdenticon';

	let {
		pubId,
		name,
		height,
		width
	}: {
		pubId: string;
		name: string;
		height?: string;
		width?: string;
		useDiceBear?: boolean;
	} = $props();
</script>

<WrappedTooltip caption={name}>
	<Avatar.Root class="{height ?? 'h-12'} {width ?? 'w-12'}">
		<Avatar.Image alt={name} src={`data:image/svg+xml;base64,${btoa(toSvg(pubId, 140))}`} />
		<Avatar.Fallback>
			{toShortInitials(name)}
		</Avatar.Fallback>
	</Avatar.Root>
</WrappedTooltip>
