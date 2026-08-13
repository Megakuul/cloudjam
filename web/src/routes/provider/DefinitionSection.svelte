<script lang="ts">
	import { Glue, Submit, toDigest } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Button } from '$lib/components/shad/button';
	import * as Table from '$lib/components/shad/table';
	import { ListRequestSchema } from '$lib/sdk/v1/cloud/definition/definition_pb';
	import type { Definition } from '$lib/sdk/v1/cloud/definition_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import { onMount } from 'svelte';

	let { providerId }: { providerId: string } = $props();

	const limit = 100;

	let error = $state('');
	let loading = $state(false);
	let forbidden = $state(false);

	let definitions: Definition[] = $state([]);
	let exhausted = $state(true);

	// definitions live in the provider partition, so listing them is a query and not a scan.
	function load(startAfter?: string) {
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
				<Table.Head>Scope</Table.Head>
			</Table.Row>
		</Table.Header>
		<Table.Body>
			{#each definitions as definition (definition.id)}
				<Table.Row>
					<Table.Cell class="font-medium">{definition.name}</Table.Cell>
					<Table.Cell>{definition.version}</Table.Cell>
					<Table.Cell>{definition.description}</Table.Cell>
					<Table.Cell class="font-mono text-xs break-all">{toDigest(definition.hash)}</Table.Cell>
					<Table.Cell>{definition.scope}</Table.Cell>
				</Table.Row>
			{:else}
				<Table.Row>
					<Table.Cell colspan={5}>
						<p class="p-4 text-sm text-muted-foreground italic">No definitions uploaded to this provider yet.</p>
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

{#if error}
	<Alert.Root variant="destructive">
		<AlertCircleIcon />
		<Alert.Title>Failed to load definitions</Alert.Title>
		<Alert.Description>{error}</Alert.Description>
	</Alert.Root>
{/if}
