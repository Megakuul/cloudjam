<script lang="ts">
	import { page } from '$app/state';

	import { Glue, Submit } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import { GetRequestSchema } from '$lib/sdk/v1/cloud/provider/provider_pb';
	import { ProviderType, type Provider } from '$lib/sdk/v1/cloud/provider_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import { onMount } from 'svelte';
	import AccountSection from './AccountSection.svelte';
	import DefinitionSection from './DefinitionSection.svelte';
	import ProviderPanel from './ProviderPanel.svelte';
	import { goto } from '$app/navigation';

	let providerId = $derived(page.params.id ?? '');

	const tabs = ['overview', 'accounts', 'definitions'] as const;

	let error = $state('');
	let forbidden = $state(false);

	let provider: Provider | undefined = $state();
	let tab: (typeof tabs)[number] = $state('overview');

	function load() {
		Submit(
			async () => {
				provider = (await Glue.provider.get(create(GetRequestSchema, { id: providerId }))).provider;
			},
			(e, _, f) => ((error = e), (forbidden = f))
		);
	}

	onMount(() => load());
</script>

<svelte:head>
	<title>Provider | CloudJam</title>
	<meta property="og:title" content="Provider | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<Button variant="ghost" size="icon" class="cursor-pointer" onclick={() => goto('/provider')}>
			<ChevronLeftIcon />
		</Button>
		<h1 class="text-3xl opacity-80">{provider?.name ?? 'Provider'}</h1>
		{#if provider}
			<Badge variant="secondary">{ProviderType[provider.type]}</Badge>
			<Badge variant="outline">scope: {provider.scope}</Badge>
		{/if}
		<Button
			variant="outline"
			class="ml-auto cursor-pointer font-mono text-xs"
			title="Copy provider id"
			onclick={() => navigator.clipboard.writeText(providerId)}
		>
			<CopyIcon />
			{providerId}
		</Button>
	</div>

	<div class="flex flex-row gap-2 border-b">
		{#each tabs as name (name)}
			<button
				type="button"
				class="cursor-pointer border-b-2 px-4 py-2 capitalize transition-all duration-200 {tab === name
					? 'border-primary'
					: 'border-transparent opacity-60 hover:opacity-100'}"
				onclick={() => (tab = name)}
			>
				{name}
			</button>
		{/each}
	</div>

	{#if forbidden}
		<Alert.Root>
			<AlertCircleIcon />
			<Alert.Title>Permission Denied</Alert.Title>
			<Alert.Description>You are not allowed to open this provider</Alert.Description>
		</Alert.Root>
	{:else if tab === 'overview'}
		{#if provider}
			<ProviderPanel {provider} refresh={() => load()} deleted={() => goto('/provider')} />
		{/if}
	{:else if tab === 'accounts'}
		<AccountSection {providerId} />
	{:else if tab === 'definitions'}
		<DefinitionSection {providerId} />
	{/if}

	{#if error}
		<Alert.Root variant="destructive">
			<AlertCircleIcon />
			<Alert.Title>Failed to load the provider</Alert.Title>
			<Alert.Description>{error}</Alert.Description>
		</Alert.Root>
	{/if}
</div>
