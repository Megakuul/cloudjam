<script lang="ts">
	import { Glue, Submit } from '$lib';
	import { toDigest } from '$lib/digest';
	import * as Alert from '$lib/components/shad/alert';
	import { Button } from '$lib/components/shad/button';
	import * as Table from '$lib/components/shad/table';
	import { ListRequestSchema } from '$lib/sdk/v1/cloud/definition/definition_pb';
	import type { Definition } from '$lib/sdk/v1/cloud/definition_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import { onMount } from 'svelte';
	import DefinitionPanel from './DefinitionPanel.svelte';

	let { providerId }: { providerId: string } = $props();

	const limit = 100;

	let error = $state('');
	// the shell is prerendered, so this starts loading: the list is only known after the
	// request that onMount fires once hydration completed.
	let loading = $state(true);
	let forbidden = $state(false);

	let definitions: Definition[] = $state([]);
	let exhausted = $state(true);

	let selected: Definition | undefined = $state();

	// definitions live in the provider partition, so listing them is a query and not a scan.
	function load(startAfter?: string) {
		selected = undefined;
		Submit(
			async () => {
				const resp = await Glue.definition.list(
					create(ListRequestSchema, { providerId: providerId, limit: limit, startAfter: startAfter })
				);
				definitions = startAfter ? [...definitions, ...resp.definitions] : resp.definitions;
				exhausted = resp.definitions.length < limit;
			},
			(e, l, f) => ((error = e), (loading = l), (forbidden = f))
		);
	}

	onMount(() => load());
</script>

<div class="flex w-full flex-col gap-4">
	<h2 class="text-xl opacity-80">Definitions on this provider</h2>

	{#if forbidden}
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
						onclick={() => (selected = selected?.id === definition.id ? undefined : definition)}
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
								{loading ? 'Loading definitions…' : 'No definitions uploaded to this provider yet.'}
							</p>
						</Table.Cell>
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
		{#if !exhausted}
			<Button
				variant="outline"
				class="cursor-pointer self-center"
				disabled={loading}
				onclick={() => load(definitions.at(-1)?.id)}
			>
				Load More
			</Button>
		{/if}
	{/if}

	{#if selected}
		{#key selected.id}
			<DefinitionPanel definition={selected} refresh={() => load()} />
		{/key}
	{/if}

	{#if error}
		<Alert.Root variant="destructive">
			<AlertCircleIcon />
			<Alert.Title>Failed to load definitions</Alert.Title>
			<Alert.Description>{error}</Alert.Description>
		</Alert.Root>
	{/if}
</div>
