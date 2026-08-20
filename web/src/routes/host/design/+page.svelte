<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import { Button } from '$lib/components/shad/button';
	import {
		GetRequestSchema as GetProviderRequestSchema,
		ListRequestSchema as ListProviderRequestSchema
	} from '$lib/sdk/v1/cloud/provider/provider_pb';
	import { create } from '@bufbuild/protobuf';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import CreateDefinition from './CreateDefinition.svelte';
	import type { Provider } from '$lib/sdk/v1/cloud/provider_pb';
	import { onMount } from 'svelte';
	import OptionalSelect from '$lib/components/custom/OptionalSelect.svelte';
	import { toDigest } from '$lib/digest';
	import * as Alert from '$lib/components/shad/alert';
	import * as Table from '$lib/components/shad/table';
	import { ListRequestSchema } from '$lib/sdk/v1/cloud/definition/definition_pb';
	import type { Definition } from '$lib/sdk/v1/cloud/definition_pb';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import DefinitionPanel from './DefinitionPanel.svelte';
	import Badge from '$lib/components/shad/badge/badge.svelte';
	import { CircleQuestionMarkIcon, WandSparklesIcon } from '@lucide/svelte';

	let providerId: string = $state('');
	let provider: Provider | undefined = $state();
	let providerState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	$effect(() => {
		Submit(async () => {
			provider = (await Glue.provider.get(create(GetProviderRequestSchema, { id: providerId }))).provider;
		}, providerState);
	});

	let providerSuggestions: Provider[] = $state([]);
	let providerSuggestionsState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	onMount(() => {
		Submit(async () => {
			providerSuggestions = (await Glue.provider.list(create(ListProviderRequestSchema, { limit: 100 }))).providers;
		}, providerSuggestionsState);
	});

	let definitions: Definition[] = $state([]);
	let definitionsState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let definitionsExhausted = $state(true);

	let selectedDefinition: Definition | undefined = $state();

	function loadDefinitions(startAfter?: string) {
		Submit(async () => {
			const resp = await Glue.definition.list(
				create(ListRequestSchema, { providerId: providerId, limit: 100, startAfter: startAfter })
			);
			definitions = startAfter ? [...definitions, ...resp.definitions] : resp.definitions;
			definitionsExhausted = resp.definitions.length < 100;
		}, definitionsState);
	}
	$effect(() => {
		if (providerId) {
			loadDefinitions();
		}
	});
</script>

<svelte:head>
	<title>Design | CloudJam</title>
	<meta property="og:title" content="Design | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<Button variant="ghost" size="icon" class="cursor-pointer" href="/host/">
			<ChevronLeftIcon />
		</Button>
		<h1 class="text-3xl opacity-80">Design</h1>
	</div>

	<p class="text-muted-foreground text-sm">Create, update, manage all challenge definitions</p>

	<div class="flex flex-row items-center gap-4">
		<OptionalSelect
			bind:value={providerId}
			placeholder="Select Provider"
			suggestions={providerSuggestions.map((provider) => ({ id: provider.id, title: provider.name }))}
			class="w-64"
		/>

		{#if provider}
			<Badge href={`/provider/${provider.id}`} variant="outline">
				{provider?.id}
			</Badge>
		{/if}

		<Button class="ml-auto" href="todo">
			<CircleQuestionMarkIcon />
			Help
		</Button>
	</div>

	{#if provider}
		<CreateDefinition {provider} refresh={() => loadDefinitions()} />

		<div class="flex w-full flex-col gap-4">
			<h2 class="text-xl opacity-80">Challenge Definitions on the Provider</h2>
			{#if definitionsState.forbidden}
				<Alert.Root>
					<AlertCircleIcon />
					<Alert.Title>Permission Denied</Alert.Title>
					<Alert.Description>You are not allowed to list the definitions of this provider</Alert.Description>
				</Alert.Root>
			{:else}
				<Table.Root>
					<Table.Header>
						<Table.Row>
							<Table.Head>Name</Table.Head>
							<Table.Head>Version</Table.Head>
							<Table.Head>Description</Table.Head>
							<Table.Head>Digest</Table.Head>
						</Table.Row>
					</Table.Header>
					<Table.Body>
						{#each definitions as definition (definition.id)}
							<Table.Row
								class="cursor-pointer"
								onclick={() => (selectedDefinition = selectedDefinition?.id === definition.id ? undefined : definition)}
							>
								<Table.Cell class="font-medium">{definition.name}</Table.Cell>
								<Table.Cell>{definition.version}</Table.Cell>
								<Table.Cell>{definition.description}</Table.Cell>
								<Table.Cell class="font-mono text-xs break-all">{toDigest(definition.hash)}</Table.Cell>
							</Table.Row>
						{:else}
							<Table.Row>
								<Table.Cell colspan={4}>
									<p class="text-muted-foreground p-4 text-sm italic">
										{definitionsState.loading
											? 'Loading definitions…'
											: 'No definitions uploaded to this provider yet.'}
									</p>
								</Table.Cell>
							</Table.Row>
						{/each}
					</Table.Body>
				</Table.Root>
				{#if !definitionsExhausted}
					<Button
						variant="outline"
						class="cursor-pointer self-center"
						disabled={definitionsState.loading}
						onclick={() => loadDefinitions(definitions.at(-1)?.id)}
					>
						Load More
					</Button>
				{/if}
			{/if}

			{#if selectedDefinition}
				{#key selectedDefinition.id}
					<DefinitionPanel
						definition={selectedDefinition}
						refresh={() => loadDefinitions()}
						close={() => (selectedDefinition = undefined)}
					/>
				{/key}
			{/if}

			{#if definitionsState.error}
				<Alert.Root variant="destructive">
					<AlertCircleIcon />
					<Alert.Title>Failed to load definitions</Alert.Title>
					<Alert.Description>{definitionsState.error}</Alert.Description>
				</Alert.Root>
			{/if}
		</div>
	{:else}
		<div class="flex h-[60vh] w-full flex-col items-center justify-center gap-8">
			<WandSparklesIcon class="text-muted h-48 w-48" />
			<h1 class="text-muted text-4xl font-bold">The Canvas is yours</h1>
		</div>
	{/if}
</div>
