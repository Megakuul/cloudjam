<script lang="ts">
	import { Glue, Submit } from '$lib';
	import { Input } from '$lib/components/shad/input';
	import { GetRequestSchema } from '$lib/sdk/v1/admin/user/user_pb';
	import { create } from '@bufbuild/protobuf';
	import { onMount } from 'svelte';

	// ScopeInput is a text input suggesting known scopes (collected from existing roles)
	// while still allowing arbitrary new scopes to be typed.
	let {
		value = $bindable(),
		scopes = [],
		...restProps
	}: { value: string; scopes?: string[]; id?: string; class?: string; placeholder?: string } = $props();

	const listId = $props.id();

	// the user service resolves the requestor when no id is given, its scope is the default.
	let own = $state('');

	onMount(() =>
		Submit(
			async () => {
				own = (await Glue.user.get(create(GetRequestSchema, {}))).user?.scope ?? '';
				if (!value) value = own;
			},
			() => {}
		)
	);
</script>

<Input bind:value list={listId} {...restProps} />
<datalist id={listId}>
	{#each [...new Set([own, ...scopes])].filter((scope) => scope) as scope (scope)}
		<option value={scope}></option>
	{/each}
</datalist>
<p class="text-xs text-muted-foreground">
	A scope is the ownership boundary of a resource: everybody holding it can see and manage the resource. You can only
	attach a scope you possess yourself{own ? `, yours is "${own}"` : ''}.
</p>
