<script lang="ts">
	import { Glue, Submit } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import { ListRequestSchema } from '$lib/sdk/v1/cloud/provider/provider_pb';
	import { ProviderType, type Provider } from '$lib/sdk/v1/cloud/provider_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import CloudIcon from '@lucide/svelte/icons/cloud';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import { onMount } from 'svelte';

	let { open }: { open: (id: string) => void } = $props();

	const limit = 100;

	let error = $state('');
	// the shell is prerendered, so this starts loading: the list is only known after the
	// request that onMount fires once hydration completed.
	let loading = $state(true);
	let forbidden = $state(false);

	let providers: Provider[] = $state([]);
	let exhausted = $state(true);

	let id = $state('');

	function load(startAfter?: string) {
		Submit(
			async () => {
				const resp = await Glue.provider.list(create(ListRequestSchema, { limit: limit, startAfter: startAfter }));
				providers = startAfter ? [...providers, ...resp.providers] : resp.providers;
				exhausted = resp.providers.length < limit;
			},
			(e, l, f) => ((error = e), (loading = l), (forbidden = f))
		);
	}

	onMount(() => load());
</script>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<h1 class="text-3xl opacity-80">Providers</h1>
		<Button variant="outline" class="ml-auto cursor-pointer" href="/provider/new/">
			<PlusIcon /> Add Provider
		</Button>
	</div>

	{#if forbidden}
		<Card.Root class="w-full">
			<Card.Header>
				<Card.Title>Open a Provider</Card.Title>
				<Card.Description>You are not allowed to list providers, open the one you work on by its id.</Card.Description>
			</Card.Header>
			<Card.Content>
				<form class="flex flex-row items-center gap-2" onsubmit={() => open(id.trim())}>
					<Input class="max-w-96" bind:value={id} placeholder="Provider id" />
					<Button type="submit" class="cursor-pointer" disabled={!id.trim()}>Open</Button>
				</form>
			</Card.Content>
		</Card.Root>
	{:else}
		<div class="flex flex-row flex-wrap gap-4">
			{#each providers as provider (provider.id)}
				<button type="button" class="w-full cursor-pointer text-left sm:w-80" onclick={() => open(provider.id)}>
					<Card.Root class="h-full transition-all duration-200 hover:bg-slate-50/5">
						<Card.Header>
							<CloudIcon class="size-8" />
							<Card.Title class="text-xl">{provider.name}</Card.Title>
							<Card.Description>{provider.description}</Card.Description>
							<div class="flex flex-row flex-wrap gap-1">
								<Badge variant="secondary">{ProviderType[provider.type]}</Badge>
								<Badge variant="outline">scope: {provider.scope}</Badge>
								{#each provider.regions as region (region)}
									<Badge variant="outline">{region}</Badge>
								{/each}
							</div>
						</Card.Header>
					</Card.Root>
				</button>
			{:else}
				<p class="text-muted-foreground text-sm italic">
					{loading
						? 'Loading providers…'
						: 'No providers yet. Add one to provision sandbox accounts and store challenge plugins.'}
				</p>
			{/each}
		</div>
		{#if !exhausted}
			<Button
				variant="outline"
				class="cursor-pointer self-center"
				disabled={loading}
				onclick={() => load(providers.at(-1)?.id)}
			>
				Load More
			</Button>
		{/if}
	{/if}

	{#if error}
		<Alert.Root variant="destructive">
			<AlertCircleIcon />
			<Alert.Title>Failed to load providers</Alert.Title>
			<Alert.Description>{error}</Alert.Description>
		</Alert.Root>
	{/if}
</div>
