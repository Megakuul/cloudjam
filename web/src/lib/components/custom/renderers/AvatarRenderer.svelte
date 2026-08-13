<script lang="ts">
	import * as Avatar from '$lib/components/shad/avatar/index';
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

	/**
	 * Convert a longer string into initials of a set length
	 * @param value The string to initialise
	 * @param length Optional, a length to cap the initials ot
	 * @returns A string that has been initialised
	 */
	function toShortInitials(value: string, length: number = 2) {
		if (value.length <= length) {
			return value.toUpperCase();
		}

		return value
			.split(' ')
			.join('')
			.substring(0, length - 1)
			.toUpperCase();
	}
</script>

<WrappedTooltip caption={name}>
	<Avatar.Root class="{height ?? 'h-12'} {width ?? 'w-12'}">
		<Avatar.Image alt={name} src={`data:image/svg+xml;base64,${btoa(toSvg(pubId, 140))}`} />
		<Avatar.Fallback>
			{toShortInitials(name)}
		</Avatar.Fallback>
	</Avatar.Root>
</WrappedTooltip>
