<script lang="ts">
	import { Glue, Submit } from '$lib';
	import { Button } from '$lib/components/shad/button';
	import { Input } from '$lib/components/shad/input';
	import * as Select from '$lib/components/shad/select';
	import { ListRequestSchema } from '$lib/sdk/v1/cloud/provider/provider_pb';
	import type { Provider } from '$lib/sdk/v1/cloud/provider_pb';
	import { create } from '@bufbuild/protobuf';
	import { onMount } from 'svelte';

	// ProviderSelect picks the provider a nested resource belongs to. Listing providers needs
	// list permission, so without it the picker falls back to entering the id by hand.
	let { value = $bindable(), onselect }: { value: string; onselect: () => void } = $props();

	let providers: Provider[] = $state([]);
	let forbidden = $state(false);
	let error = $state('');
	let id = $state('');

	onMount(() =>
		Submit(
			async () => {
				providers = (await Glue.provider.list(create(ListRequestSchema, { limit: 100 }))).providers;
				value = providers[0]?.id ?? '';
				if (value) onselect();
			},
			(e, _, f) => ((error = e), (forbidden = f))
		)
	);
</script>

<div class="flex flex-col gap-1">
	{#if forbidden}
		<form class="flex flex-row items-center gap-2" onsubmit={() => ((value = id.trim()), value && onselect())}>
			<Input class="max-w-96" bind:value={id} placeholder="Provider id" />
			<Button type="submit" variant="outline" class="cursor-pointer" disabled={!id.trim()}>Open</Button>
		</form>
		<p class="text-xs text-muted-foreground">
			You are not allowed to list providers, open the one you work on by its id.
		</p>
	{:else}
		<Select.Root type="single" bind:value onValueChange={() => onselect()}>
			<Select.Trigger class="w-72 cursor-pointer">
				{providers.find((provider) => provider.id === value)?.name ?? 'Select a provider'}
			</Select.Trigger>
			<Select.Content>
				{#each providers as provider (provider.id)}
					<Select.Item value={provider.id} label={provider.name}>{provider.name}</Select.Item>
				{/each}
			</Select.Content>
		</Select.Root>
		{#if error}
			<p class="text-xs text-destructive">{error}</p>
		{/if}
	{/if}
</div>
