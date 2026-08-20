<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Table from '$lib/components/shad/table';
	import { ListRequestSchema } from '$lib/sdk/v1/admin/role/role_pb';
	import type { Role } from '$lib/sdk/v1/admin/role_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import { onMount } from 'svelte';
	import RolePanel from './RolePanel.svelte';

	const limit = 100;

	let listState: SubmitState = $state({ loading: false, error: '', forbidden: false });

	let roles: Role[] = $state([]);
	let exhausted = $state(true);

	let selected: Role | undefined = $state();

	function load() {
		selected = undefined;
		Submit(async () => {
			const resp = await Glue.role.list(create(ListRequestSchema, { limit: limit }));
			roles = resp.roles;
			exhausted = resp.roles.length < limit;
		}, listState);
	}

	onMount(() => load());
</script>

<svelte:head>
	<title>Roles | CloudJam</title>
	<meta property="og:title" content="Roles | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<Button variant="ghost" size="icon" class="cursor-pointer" href="/admin/">
			<ChevronLeftIcon />
		</Button>
		<h1 class="text-3xl opacity-80">Roles</h1>
		<Button variant="outline" class="ml-auto cursor-pointer" href="/admin/roles/new/">
			<PlusIcon /> Create Role
		</Button>
	</div>

	{#if listState.forbidden}
		<Alert.Root>
			<AlertCircleIcon />
			<Alert.Title>Permission Denied</Alert.Title>
			<Alert.Description>You are not allowed to list roles in your scope</Alert.Description>
		</Alert.Root>
	{:else}
		<Table.Root>
			<Table.Header>
				<Table.Row>
					<Table.Head>Name</Table.Head>
					<Table.Head>Scope</Table.Head>
					<Table.Head>Permissions</Table.Head>
					<Table.Head>Id</Table.Head>
					<Table.Head></Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each roles as role (role.id)}
					<Table.Row class="cursor-pointer" onclick={() => (selected = selected?.id === role.id ? undefined : role)}>
						<Table.Cell class="font-medium">{role.name}</Table.Cell>
						<Table.Cell>{role.scope}</Table.Cell>
						<Table.Cell>{Object.keys(role.permissions).length} scope(s)</Table.Cell>
						<Table.Cell class="font-mono text-xs">{role.id}</Table.Cell>
						<Table.Cell>
							{#if role.builtin}
								<Badge variant="secondary">builtin</Badge>
							{/if}
						</Table.Cell>
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
		{#if !exhausted}
			<Button
				variant="outline"
				class="cursor-pointer self-center"
				disabled={listState.loading}
				onclick={() =>
					Submit(async () => {
						const resp = await Glue.role.list(
							create(ListRequestSchema, { limit: limit, startAfter: roles.at(-1)?.id })
						);
						roles = [...roles, ...resp.roles];
						exhausted = resp.roles.length < limit;
					}, listState)}
			>
				Load More
			</Button>
		{/if}
	{/if}

	{#if selected}
		{#key selected.id}
			<RolePanel role={selected} refresh={() => load()} />
		{/key}
	{/if}

	{#if listState.error}
		<Alert.Root variant="destructive">
			<AlertCircleIcon />
			<Alert.Title>Failed to load roles</Alert.Title>
			<Alert.Description>{listState.error}</Alert.Description>
		</Alert.Root>
	{/if}
</div>
