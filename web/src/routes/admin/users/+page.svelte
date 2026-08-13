<script lang="ts">
	import { Glue, Submit } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Table from '$lib/components/shad/table';
	import { ListRequestSchema as ListRolesRequestSchema } from '$lib/sdk/v1/admin/role/role_pb';
	import type { Role } from '$lib/sdk/v1/admin/role_pb';
	import { ListRequestSchema } from '$lib/sdk/v1/admin/user/user_pb';
	import type { User } from '$lib/sdk/v1/admin/user_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import { toSvg } from 'jdenticon';
	import { onMount } from 'svelte';
	import { Icon } from 'svelte-ux';
	import UserPanel from './UserPanel.svelte';

	const limit = 100;

	let error = $state('');
	let loading = $state(false);
	let forbidden = $state(false);

	let users: User[] = $state([]);
	let exhausted = $state(true);

	let roles: Role[] = $state([]);
	let rolesForbidden = $state(false);

	let selected: User | undefined = $state();

	// scopes already known from the existing roles, used as suggestions for scope inputs.
	let scopes = $derived(
		[...new Set(roles.flatMap((role) => [role.scope, ...Object.keys(role.permissions)]))].filter((s) => s).sort()
	);

	function roleName(id: string): string {
		return roles.find((role) => role.id === id)?.name ?? id;
	}

	function load() {
		selected = undefined;
		Submit(
			async () => {
				const resp = await Glue.user.list(create(ListRequestSchema, { limit: limit }));
				users = resp.users;
				exhausted = resp.users.length < limit;
			},
			(e, l, f) => ((error = e), (loading = l), (forbidden = f))
		);
	}

	onMount(() => {
		load();
		// roles are only used to resolve names and attach them to users; if the requestor
		// has no role access the section is simply hidden and raw role ids are shown.
		Submit(
			async () => {
				roles = (await Glue.role.list(create(ListRolesRequestSchema, { limit: limit }))).roles;
			},
			(_, __, f) => (rolesForbidden = f)
		);
	});
</script>

<svelte:head>
	<title>Users | CloudJam</title>
	<meta property="og:title" content="Users | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<Button variant="ghost" size="icon" class="cursor-pointer" href="/admin/">
			<ChevronLeftIcon />
		</Button>
		<h1 class="text-3xl opacity-80">Users</h1>
		<Button variant="outline" class="ml-auto cursor-pointer" href="/admin/users/new/">
			<PlusIcon /> Invite User
		</Button>
	</div>

	{#if forbidden}
		<Alert.Root>
			<AlertCircleIcon />
			<Alert.Title>Permission Denied</Alert.Title>
			<Alert.Description>You are not allowed to list users in your scope</Alert.Description>
		</Alert.Root>
	{:else}
		<Table.Root>
			<Table.Header>
				<Table.Row>
					<Table.Head></Table.Head>
					<Table.Head>Username</Table.Head>
					<Table.Head>Email</Table.Head>
					<Table.Head>Organization</Table.Head>
					<Table.Head>Role</Table.Head>
					<Table.Head>Created</Table.Head>
					<Table.Head></Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each users as user (user.id)}
					<Table.Row class="cursor-pointer" onclick={() => (selected = selected?.id === user.id ? undefined : user)}>
						<Table.Cell>
							<Icon svg={toSvg(user.pubId, 20)} width="2rem" height="2rem" class="rounded-md bg-primary/5" />
						</Table.Cell>
						<Table.Cell class="font-medium">{user.username}</Table.Cell>
						<Table.Cell>{user.email}</Table.Cell>
						<Table.Cell>{user.organization}</Table.Cell>
						<Table.Cell>{roleName(user.role)}</Table.Cell>
						<Table.Cell>{new Date(Number(user.createdAt) * 1000).toLocaleDateString()}</Table.Cell>
						<Table.Cell>
							{#if user.privileged}
								<Badge variant="secondary">privileged</Badge>
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
				disabled={loading}
				onclick={() =>
					Submit(
						async () => {
							const resp = await Glue.user.list(
								create(ListRequestSchema, { limit: limit, startAfter: users.at(-1)?.id })
							);
							users = [...users, ...resp.users];
							exhausted = resp.users.length < limit;
						},
						(e, l, f) => ((error = e), (loading = l), (forbidden = f))
					)}
			>
				Load More
			</Button>
		{/if}
	{/if}

	{#if selected}
		{#key selected.id}
			<UserPanel user={selected} {roles} {rolesForbidden} {scopes} refresh={() => load()} />
		{/key}
	{/if}

	{#if error}
		<Alert.Root variant="destructive">
			<AlertCircleIcon />
			<Alert.Title>Failed to load users</Alert.Title>
			<Alert.Description>{error}</Alert.Description>
		</Alert.Root>
	{/if}
</div>
