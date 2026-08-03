<script lang="ts">
	import { Glue, Submit } from '$lib';
	import ScopeInput from '$lib/components/custom/ScopeInput.svelte';
	import * as Alert from '$lib/components/ui/alert';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Separator } from '$lib/components/ui/separator';
	import { AttachScopeRequestSchema, ConfigureRoleRequestSchema, Resource } from '$lib/sdk/v1/admin/rbac/rbac_pb';
	import { DeleteRequestSchema, GetRequestSchema, UpdateRequestSchema } from '$lib/sdk/v1/admin/role/role_pb';
	import { RoleSchema, type Role } from '$lib/sdk/v1/admin/role_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import { onMount } from 'svelte';
	import PermissionEditor from './PermissionEditor.svelte';

	let { role, scopes, refresh }: { role: Role; scopes: string[]; refresh: () => void } = $props();

	type section = { error: string; loading: boolean; forbidden: boolean };
	const setter = (s: section) => (e: string, l: boolean, f: boolean) => (
		(s.error = e),
		(s.loading = l),
		(s.forbidden = f)
	);

	// the panel is remounted (keyed) per role, capturing the initial value is intended.
	// svelte-ignore state_referenced_locally
	let name = $state(role.name);
	// svelte-ignore state_referenced_locally
	let description = $state(role.description);
	let attachScope = $state('');
	let confirmDelete = $state(false);
	// svelte-ignore state_referenced_locally
	let entries: { scope: string; patterns: string }[] = $state(
		Object.entries(role.permissions).map(([scope, patterns]) => ({ scope, patterns }))
	);

	let update: section = $state({ error: '', loading: false, forbidden: false });
	let configure: section = $state({ error: '', loading: false, forbidden: false });
	let attach: section = $state({ error: '', loading: false, forbidden: false });
	let remove: section = $state({ error: '', loading: false, forbidden: false });

	let mod = $derived(
		create(RoleSchema, {
			...role,
			name: name,
			description: description,
			permissions: Object.fromEntries(entries.filter((e) => e.scope).map((e) => [e.scope, e.patterns]))
		})
	);

	onMount(() => {
		// the list output does not carry the description, it is enriched by a get request.
		Submit(
			async () => {
				description = (await Glue.role.get(create(GetRequestSchema, { id: role.id }))).role?.description ?? '';
			},
			() => {}
		);
	});
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title class="flex flex-row items-center gap-2 text-2xl">
			{role.name}
			{#if role.builtin}
				<Badge variant="secondary">builtin</Badge>
			{/if}
			{#if role.scope}
				<Badge variant="outline">scope: {role.scope}</Badge>
			{/if}
		</Card.Title>
		<Card.Description class="font-mono text-xs">{role.id}</Card.Description>
	</Card.Header>
	<Card.Content class="flex flex-col gap-6">
		{#if role.builtin}
			<p class="text-sm text-muted-foreground italic">
				This role is builtin; its metadata and permissions cannot be modified.
			</p>
		{:else}
			<div class="flex flex-col gap-2">
				<Card.Title>Metadata</Card.Title>
				{#if update.forbidden}
					<p class="text-sm text-muted-foreground italic">You are not allowed to update this role.</p>
				{:else}
					<form
						class="flex flex-col gap-2"
						onsubmit={() =>
							Submit(async () => {
								await Glue.role.update(create(UpdateRequestSchema, { mod: mod }));
								refresh();
							}, setter(update))}
					>
						<div class="flex flex-row items-center gap-2">
							<Input class="max-w-48" bind:value={name} placeholder="Name of the role" />
							<Input class="max-w-96" bind:value={description} placeholder="Description of the role" />
							<Button
								type="submit"
								variant="outline"
								class="cursor-pointer"
								disabled={update.loading || Boolean(Glue.Validate(RoleSchema, mod).violation.name)}
							>
								Save
							</Button>
						</div>
						<p class="text-xs text-destructive">{Glue.Validate(RoleSchema, mod).violation.name ?? ''}</p>
						{#if update.error}
							<p class="text-xs text-destructive">{update.error}</p>
						{/if}
					</form>
				{/if}
			</div>

			<Separator />

			<div class="flex flex-col gap-2">
				<Card.Title>Permissions</Card.Title>
				{#if configure.forbidden}
					<p class="text-sm text-muted-foreground italic">You are not allowed to configure this role.</p>
				{:else}
					<p class="text-sm text-muted-foreground">
						Comma separated glob patterns matched against the rpc procedure names, granted to subjects of the entries
						scope. You can only grant scopes you possess yourself.
					</p>
					<PermissionEditor bind:entries {scopes} />
					<Button
						variant="outline"
						class="cursor-pointer self-start"
						disabled={configure.loading || Boolean(Glue.Validate(RoleSchema, mod).violation.permissions)}
						onclick={() =>
							Submit(async () => {
								await Glue.rbac.configureRole(create(ConfigureRoleRequestSchema, { mod: mod }));
								refresh();
							}, setter(configure))}
					>
						Save Permissions
					</Button>
					{#if configure.error}
						<p class="text-xs text-destructive">{configure.error}</p>
					{/if}
				{/if}
			</div>

			<Separator />
		{/if}

		<div class="flex flex-col gap-2">
			<Card.Title>Scope</Card.Title>
			{#if attach.forbidden}
				<p class="text-sm text-muted-foreground italic">You are not allowed to attach scopes.</p>
			{:else}
				<p class="text-sm text-muted-foreground">Moves this role into another scope you possess.</p>
				<div class="flex flex-row items-center gap-2">
					<ScopeInput class="max-w-48" bind:value={attachScope} {scopes} placeholder="New scope" />
					<Button
						variant="outline"
						class="cursor-pointer"
						disabled={attach.loading || !attachScope}
						onclick={() =>
							Submit(async () => {
								await Glue.rbac.attachScope(
									create(AttachScopeRequestSchema, {
										resource: Resource.RoleData,
										id: role.id,
										scope: attachScope
									})
								);
								refresh();
							}, setter(attach))}
					>
						Attach
					</Button>
				</div>
				{#if attach.error}
					<p class="text-xs text-destructive">{attach.error}</p>
				{/if}
			{/if}
		</div>

		{#if !role.builtin}
			<Separator />

			<div class="flex flex-col gap-2">
				<Card.Title>Danger Zone</Card.Title>
				{#if remove.forbidden}
					<p class="text-sm text-muted-foreground italic">You are not allowed to delete this role.</p>
				{:else}
					<div class="flex flex-row items-center gap-2">
						{#if confirmDelete}
							<Button
								variant="destructive"
								class="cursor-pointer"
								disabled={remove.loading}
								onclick={() =>
									Submit(async () => {
										await Glue.role.delete(create(DeleteRequestSchema, { id: role.id }));
										refresh();
									}, setter(remove))}
							>
								Yes, delete {role.name}
							</Button>
							<Button variant="outline" class="cursor-pointer" onclick={() => (confirmDelete = false)}>Cancel</Button>
						{:else}
							<Button variant="destructive" class="cursor-pointer" onclick={() => (confirmDelete = true)}>
								Delete Role
							</Button>
						{/if}
					</div>
					{#if remove.error}
						<Alert.Root variant="destructive">
							<AlertCircleIcon />
							<Alert.Title>Failed to delete role</Alert.Title>
							<Alert.Description>{remove.error}</Alert.Description>
						</Alert.Root>
					{/if}
				{/if}
			</div>
		{/if}
	</Card.Content>
</Card.Root>
