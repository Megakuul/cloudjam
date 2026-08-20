<script lang="ts">
	import { Input } from '$lib/components/shad/input';
	import * as Select from '$lib/components/shad/select';

	export type Suggestion = { id: string; title: string };

	let {
		value = $bindable(),
		suggestions,
		placeholder,
		onselect,
		...restProps
	}: {
		value: string;
		suggestions: Suggestion[];
		placeholder: string;
		onselect?: () => void;
		class?: string;
	} = $props();
</script>

{#if suggestions.length < 1}
	<Input class="max-w-96" bind:value {placeholder} {...restProps} />
{:else}
	<Select.Root type="single" bind:value onValueChange={() => onselect?.()}>
		<Select.Trigger class="w-72 cursor-pointer" {...restProps}>
			{suggestions.find((suggestion) => suggestion.id === value)?.title ?? placeholder}
		</Select.Trigger>
		<Select.Content>
			{#each suggestions as suggestion (suggestion.id)}
				<Select.Item value={suggestion.id} label={suggestion.title}>{suggestion.title}</Select.Item>
			{/each}
		</Select.Content>
	</Select.Root>
{/if}
