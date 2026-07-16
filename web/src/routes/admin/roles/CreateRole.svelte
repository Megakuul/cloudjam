<script lang="ts">
	import { Glue, Submit } from '$lib';
	import ScopeInput from '$lib/components/ScopeInput.svelte';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { ConfigureRoleRequestSchema } from '$lib/sdk/v1/admin/rbac/rbac_pb';
	import { CreateRequestSchema } from '$lib/sdk/v1/admin/role/role_pb';
	import { RoleSchema } from '$lib/sdk/v1/admin/role_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import PermissionEditor from './PermissionEditor.svelte';

	let { scopes, oncreated }: { scopes: string[]; oncreated: () => void } = $props();

	let error = $state('');
	let loading = $state(false);
	let forbidden = $state(false);

	let id = $state(crypto.randomUUID());
	let name = $state('');
	let description = $state('');
	let scope = $state('');
	let entries: { scope: string; patterns: string }[] = $state([{ scope: '', patterns: '' }]);

	let init = $derived(
		create(RoleSchema, {
			id: id,
			name: name,
			description: description,
			scope: scope,
			permissions: Object.fromEntries(entries.filter((e) => e.scope).map((e) => [e.scope, e.patterns]))
		})
	);
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title>Create Role</Card.Title>
		<Card.Description>
			Creates a new role in the provided scope; permissions grant access by glob matching rpc procedure names against
			the patterns of every scope the requestor possesses.
		</Card.Description>
	</Card.Header>
	<Card.Content>
		{#if forbidden}
			<p class="text-sm text-muted-foreground italic">You are not allowed to create roles in your scope.</p>
		{:else}
			<form
				class="flex flex-col gap-4"
				onsubmit={() =>
					Submit(
						async () => {
							await Glue.role.create(create(CreateRequestSchema, { init: init }));
							// create only persists the metadata, the permission map
							// is configured over the dedicated rbac endpoint.
							await Glue.rbac.configureRole(create(ConfigureRoleRequestSchema, { mod: init }));
							id = crypto.randomUUID();
							name = description = scope = '';
							entries = [{ scope: '', patterns: '' }];
							oncreated();
						},
						(e, l, f) => ((error = e), (loading = l), (forbidden = f))
					)}
			>
				<div class="grid gap-4 md:grid-cols-2">
					<div class="flex flex-col gap-1">
						<label for="role-name" class="text-sm">Name</label>
						<Input id="role-name" bind:value={name} placeholder="Name of the new role" />
						<p class="text-xs text-destructive">{Glue.Validate(RoleSchema, init).violation.name ?? ''}</p>
					</div>
					<div class="flex flex-col gap-1">
						<label for="role-description" class="text-sm">Description</label>
						<Input id="role-description" bind:value={description} placeholder="Description of the new role" />
						<p class="text-xs text-destructive">{Glue.Validate(RoleSchema, init).violation.description ?? ''}</p>
					</div>
					<div class="flex flex-col gap-1">
						<label for="role-scope" class="text-sm">Scope</label>
						<ScopeInput id="role-scope" bind:value={scope} {scopes} placeholder="Scope the role is placed in" />
						<p class="text-xs text-muted-foreground">You can only attach a scope you possess yourself.</p>
					</div>
				</div>
				<div class="flex flex-col gap-1">
					<span class="text-sm">Permissions</span>
					<PermissionEditor bind:entries {scopes} />
				</div>
				<Button
					type="submit"
					class="cursor-pointer self-start"
					disabled={loading || Boolean(Glue.Validate(RoleSchema, init).error)}
				>
					Create
				</Button>
				{#if error}
					<Alert.Root variant="destructive">
						<AlertCircleIcon />
						<Alert.Title>Failed to create role</Alert.Title>
						<Alert.Description>{error}</Alert.Description>
					</Alert.Root>
				{/if}
			</form>
		{/if}
	</Card.Content>
</Card.Root>
