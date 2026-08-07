<script lang="ts">
	import { Input } from '$lib/components/shad/input';

	// ScopeInput is a text input suggesting known scopes (collected from existing roles)
	// while still allowing arbitrary new scopes to be typed.
	let {
		value = $bindable(),
		scopes,
		...restProps
	}: { value: string; scopes: string[]; id?: string; class?: string; placeholder?: string } = $props();

	const listId = $props.id();
</script>

<Input bind:value list={listId} {...restProps} />
<datalist id={listId}>
	{#each scopes as scope (scope)}
		<option value={scope}></option>
	{/each}
</datalist>
