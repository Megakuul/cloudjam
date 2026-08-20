<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import { Separator } from '$lib/components/shad/separator';
	import { AttachScopeRequestSchema, ConfigureRoleRequestSchema, Resource } from '$lib/sdk/v1/admin/rbac/rbac_pb';
	import { DeleteRequestSchema, UpdateRequestSchema } from '$lib/sdk/v1/admin/role/role_pb';
	import { RoleSchema, type Role } from '$lib/sdk/v1/admin/role_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import PermissionEditor from './PermissionEditor.svelte';
	import OptionalSelect from '$lib/components/custom/OptionalSelect.svelte';
	import { scopes } from '$lib/scopes.svelte';

	let { role, refresh }: { role: Role; refresh: () => void } = $props();

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

	let updateState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let configureState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let attachState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let removeState: SubmitState = $state({ error: '', loading: false, forbidden: false });

	let mod = $derived(
		create(RoleSchema, {
			...role,
			name: name,
			description: description,
			permissions: Object.fromEntries(entries.filter((e) => e.scope).map((e) => [e.scope, e.patterns]))
		})
	);
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
			<p class="text-muted-foreground text-sm italic">
				This role is builtin; its metadata and permissions cannot be modified.
			</p>
		{:else}
			<div class="flex flex-col gap-2">
				<Card.Title>Metadata</Card.Title>
				{#if updateState.forbidden}
					<p class="text-muted-foreground text-sm italic">You are not allowed to update this role.</p>
				{:else}
					<form
						class="flex flex-col gap-2"
						onsubmit={() =>
							Submit(async () => {
								await Glue.role.update(create(UpdateRequestSchema, { mod: mod }));
								refresh();
							}, updateState)}
					>
						<div class="flex flex-row items-center gap-2">
							<Input class="max-w-48" bind:value={name} placeholder="Name of the role" />
							<Input class="max-w-96" bind:value={description} placeholder="Description of the role" />
							<Button
								type="submit"
								variant="outline"
								class="cursor-pointer"
								disabled={updateState.loading || Boolean(Glue.Validate(RoleSchema, mod).violation.name)}
							>
								Save
							</Button>
						</div>
						<p class="text-destructive text-xs">{Glue.Validate(RoleSchema, mod).violation.name ?? ''}</p>
						{#if updateState.error}
							<p class="text-destructive text-xs">{updateState.error}</p>
						{/if}
					</form>
				{/if}
			</div>

			<Separator />

			<div class="flex flex-col gap-2">
				<Card.Title>Permissions</Card.Title>
				{#if configureState.forbidden}
					<p class="text-muted-foreground text-sm italic">You are not allowed to configure this role.</p>
				{:else}
					<p class="text-muted-foreground text-sm">
						Comma separated glob patterns matched against the rpc procedure names, granted to subjects of the entries
						scope. You can only grant scopes you possess yourself.
					</p>
					<PermissionEditor bind:entries />
					<Button
						variant="outline"
						class="cursor-pointer self-start"
						disabled={configureState.loading || Boolean(Glue.Validate(RoleSchema, mod).violation.permissions)}
						onclick={() =>
							Submit(async () => {
								await Glue.rbac.configureRole(create(ConfigureRoleRequestSchema, { mod: mod }));
								refresh();
							}, configureState)}
					>
						Save Permissions
					</Button>
					{#if configureState.error}
						<p class="text-destructive text-xs">{configureState.error}</p>
					{/if}
				{/if}
			</div>

			<Separator />
		{/if}

		<div class="flex flex-col gap-2">
			<Card.Title>Scope</Card.Title>
			{#if attachState.forbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to attach scopes.</p>
			{:else}
				<p class="text-muted-foreground text-sm">Moves this role into another scope you possess.</p>
				<div class="flex flex-row items-center gap-2">
					<OptionalSelect
						bind:value={attachScope}
						placeholder="New scope"
						suggestions={scopes.map((scope) => ({ id: scope, title: scope }))}
					/>
					<Button
						variant="outline"
						class="cursor-pointer"
						disabled={attachState.loading || !attachScope}
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
							}, attachState)}
					>
						Attach
					</Button>
				</div>
				{#if attachState.error}
					<p class="text-destructive text-xs">{attachState.error}</p>
				{/if}
			{/if}
		</div>

		{#if !role.builtin}
			<Separator />

			<div class="flex flex-col gap-2">
				<Card.Title>Danger Zone</Card.Title>
				{#if removeState.forbidden}
					<p class="text-muted-foreground text-sm italic">You are not allowed to delete this role.</p>
				{:else}
					<div class="flex flex-row items-center gap-2">
						{#if confirmDelete}
							<Button
								variant="destructive"
								class="cursor-pointer"
								disabled={removeState.loading}
								onclick={() =>
									Submit(async () => {
										await Glue.role.delete(create(DeleteRequestSchema, { id: role.id }));
										refresh();
									}, removeState)}
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
					{#if removeState.error}
						<Alert.Root variant="destructive">
							<AlertCircleIcon />
							<Alert.Title>Failed to delete role</Alert.Title>
							<Alert.Description>{removeState.error}</Alert.Description>
						</Alert.Root>
					{/if}
				{/if}
			</div>
		{/if}
	</Card.Content>
</Card.Root>
