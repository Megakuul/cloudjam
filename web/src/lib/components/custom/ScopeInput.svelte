<script lang="ts">
	import { getSelf } from '$lib';
	import { Input } from '$lib/components/shad/input';
	import { onMount } from 'svelte';

	// ScopeInput is a text input suggesting known scopes (collected from existing resources)
	// while still allowing arbitrary new scopes to be typed. An empty input defaults to the
	// scope of the requesting user, the one scope they are guaranteed to possess.
	let {
		value = $bindable(),
		scopes = [],
		...restProps
	}: { value: string; scopes?: string[]; id?: string; class?: string; placeholder?: string } = $props();

	const listId = $props.id();

	let own = $state('');
	let suggestions = $derived([...new Set([own, ...scopes])].filter((scope) => scope).sort());

	onMount(async () => {
		own = (await getSelf())?.scope ?? '';
		if (!value) value = own;
	});
</script>

<Input bind:value list={listId} {...restProps} />
<datalist id={listId}>
	{#each suggestions as scope (scope)}
		<option value={scope}></option>
	{/each}
</datalist>
<p class="text-muted-foreground text-xs">
	A scope is the ownership boundary of a resource: everybody holding it can see and manage the resource. You can only
	attach a scope you possess yourself{own ? `, yours is "${own}"` : ''}.
</p>
