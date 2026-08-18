<script lang="ts">
	import { cn, type WithElementRef } from '$lib/utils.js';
	import type { HTMLInputAttributes } from 'svelte/elements';
	import Input from '../input/input.svelte';

	type Props = WithElementRef<
		Omit<HTMLInputAttributes, 'type'> & { label: string; validation: string | undefined; placeholder: string }
	>;

	let { value = $bindable(), label, validation, placeholder, class: className }: Props = $props();
</script>

<div class={cn('flex flex-col gap-1', className)}>
	<label class="text-sm">
		{label}
		<Input bind:value {placeholder} />
	</label>
	{#if validation && value}
		<p class="text-destructive text-xs">{validation}</p>
	{/if}
</div>
