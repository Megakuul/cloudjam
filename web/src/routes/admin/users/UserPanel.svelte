<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import OptionalSelect from '$lib/components/custom/OptionalSelect.svelte';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import * as Select from '$lib/components/shad/select';
	import { Separator } from '$lib/components/shad/separator';
	import { scopes } from '$lib/scopes.svelte';
	import { AttachRoleRequestSchema, AttachScopeRequestSchema, Resource } from '$lib/sdk/v1/admin/rbac/rbac_pb';
	import type { Role } from '$lib/sdk/v1/admin/role_pb';
	import { DeleteRequestSchema, ResetPasswordRequestSchema, UpdateRequestSchema } from '$lib/sdk/v1/admin/user/user_pb';
	import type { User } from '$lib/sdk/v1/admin/user_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampFromDate } from '@bufbuild/protobuf/wkt';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import { toSvg } from 'jdenticon';
	import { Icon } from 'svelte-ux';

	let {
		user,
		roles,
		rolesForbidden,
		refresh
	}: { user: User; roles: Role[]; rolesForbidden: boolean; refresh: () => void } = $props();

	// svelte-ignore state_referenced_locally
	let organization = $state(user.organization);
	// svelte-ignore state_referenced_locally
	let role = $state(user.role);
	let attachScope = $state('');
	let attachResource = $state('user');
	let resetCode = $state('');
	let confirmDelete = $state(false);

	let updateState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let attachRoleState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let attachState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let resetState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let removeState: SubmitState = $state({ error: '', loading: false, forbidden: false });

	const resources = [
		{ value: 'user', label: 'User data' },
		{ value: 'creds', label: 'Login credentials' }
	];
</script>

<Card.Root class="w-full">
	<Card.Header class="flex flex-row items-center gap-4">
		<Icon svg={toSvg(user.pubId, 20)} width="4rem" height="4rem" class="bg-primary/5 rounded-lg" />
		<div class="flex flex-col gap-1">
			<Card.Title class="text-2xl">{user.username}</Card.Title>
			<Card.Description>{user.email}</Card.Description>
			<div class="flex flex-row flex-wrap gap-1">
				{#if user.privileged}
					<Badge variant="secondary">privileged</Badge>
				{/if}
				{#if user.scope}
					<Badge variant="outline">scope: {user.scope}</Badge>
				{/if}
				{#if user.role}
					<Badge variant="outline">role: {roles.find((r) => r.id === user.role)?.name ?? user.role}</Badge>
				{/if}
			</div>
		</div>
	</Card.Header>
	<Card.Content class="flex flex-col gap-6">
		{#if user.privileged}
			<p class="text-muted-foreground text-sm italic">
				This user is privileged; role, scope and profile cannot be modified.
			</p>
		{:else}
			<div class="flex flex-col gap-2">
				<Card.Title>Organization</Card.Title>
				{#if updateState.forbidden}
					<p class="text-muted-foreground text-sm italic">You are not allowed to update this user.</p>
				{:else}
					<form
						class="flex flex-row items-center gap-2"
						onsubmit={() =>
							Submit(async () => {
								await Glue.user.update(create(UpdateRequestSchema, { mod: { ...user, organization: organization } }));
								refresh();
							}, updateState)}
					>
						<Input class="max-w-96" bind:value={organization} placeholder="Organization of the user" />
						<Button type="submit" variant="outline" class="cursor-pointer" disabled={updateState.loading}>Save</Button>
					</form>
					{#if updateState.error}
						<p class="text-destructive text-xs">{updateState.error}</p>
					{/if}
				{/if}
			</div>

			<Separator />

			<div class="flex flex-col gap-2">
				<Card.Title>Role</Card.Title>
				{#if rolesForbidden || attachRoleState.forbidden}
					<p class="text-muted-foreground text-sm italic">You are not allowed to attach roles.</p>
				{:else}
					<div class="flex flex-row items-center gap-2">
						<Select.Root type="single" bind:value={role}>
							<Select.Trigger class="w-96 cursor-pointer">
								{roles.find((r) => r.id === role)?.name ?? 'Select a role'}
							</Select.Trigger>
							<Select.Content>
								{#each roles as item (item.id)}
									<Select.Item value={item.id} label={item.name}>{item.name}</Select.Item>
								{/each}
							</Select.Content>
						</Select.Root>
						<Button
							variant="outline"
							class="cursor-pointer"
							disabled={attachRoleState.loading || !role || role === user.role}
							onclick={() =>
								Submit(async () => {
									await Glue.rbac.attachRole(create(AttachRoleRequestSchema, { id: user.id, role: role }));
									refresh();
								}, attachRoleState)}
						>
							Attach
						</Button>
					</div>
					{#if attachRoleState.error}
						<p class="text-destructive text-xs">{attachRoleState.error}</p>
					{/if}
				{/if}
			</div>

			<Separator />

			<div class="flex flex-col gap-2">
				<Card.Title>Scope</Card.Title>
				{#if attachState.forbidden}
					<p class="text-muted-foreground text-sm italic">You are not allowed to attach scopes.</p>
				{:else}
					<p class="text-muted-foreground text-sm">
						Moves the selected resource of this user into another scope you possess. User data is keyed by the user id,
						login credentials by the email.
					</p>
					<div class="flex flex-row items-center gap-2">
						<Select.Root type="single" bind:value={attachResource}>
							<Select.Trigger class="w-48 cursor-pointer">
								{resources.find((r) => r.value === attachResource)?.label}
							</Select.Trigger>
							<Select.Content>
								{#each resources as item (item.value)}
									<Select.Item value={item.value} label={item.label}>{item.label}</Select.Item>
								{/each}
							</Select.Content>
						</Select.Root>
						<OptionalSelect
							bind:value={attachScope}
							placeholder="Scope the role is placed in"
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
											resource: attachResource === 'creds' ? Resource.CredsData : Resource.UserData,
											id: attachResource === 'creds' ? user.email : user.id,
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

			<Separator />
		{/if}

		<div class="flex flex-col gap-2">
			<Card.Title>Reset Password</Card.Title>
			{#if resetState.forbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to reset passwords.</p>
			{:else if resetCode}
				<Alert.Root>
					<Alert.Title>Reset code created</Alert.Title>
					<Alert.Description class="flex flex-row items-center gap-2 font-mono text-xs break-all">
						{resetCode}
						<Button
							variant="outline"
							size="icon"
							title="Copy code"
							class="cursor-pointer"
							onclick={() => navigator.clipboard.writeText(resetCode)}
						>
							<CopyIcon />
						</Button>
					</Alert.Description>
				</Alert.Root>
			{:else}
				<div class="flex flex-row items-center gap-2">
					<Button
						variant="outline"
						class="cursor-pointer"
						disabled={resetState.loading}
						onclick={() =>
							Submit(async () => {
								const resp = await Glue.user.resetPassword(
									create(ResetPasswordRequestSchema, {
										email: user.email,
										expires: timestampFromDate(new Date(Date.now() + 24 * 60 * 60 * 1000)) // expires in 24 hours
									})
								);
								resetCode = resp.code;
							}, resetState)}
					>
						Create Reset Code
					</Button>
					<p class="text-muted-foreground text-sm">
						Creates a one time code (valid 24 hours) to redo the registration.
					</p>
				</div>
				{#if resetState.error}
					<p class="text-destructive text-xs">{resetState.error}</p>
				{/if}
			{/if}
		</div>

		{#if !user.privileged}
			<Separator />

			<div class="flex flex-col gap-2">
				<Card.Title>Danger Zone</Card.Title>
				{#if removeState.forbidden}
					<p class="text-muted-foreground text-sm italic">You are not allowed to delete this user.</p>
				{:else}
					<div class="flex flex-row items-center gap-2">
						{#if confirmDelete}
							<Button
								variant="destructive"
								class="cursor-pointer"
								disabled={removeState.loading}
								onclick={() =>
									Submit(async () => {
										await Glue.user.delete(create(DeleteRequestSchema, { id: user.id }));
										refresh();
									}, removeState)}
							>
								Yes, delete {user.username}
							</Button>
							<Button variant="outline" class="cursor-pointer" onclick={() => (confirmDelete = false)}>Cancel</Button>
						{:else}
							<Button variant="destructive" class="cursor-pointer" onclick={() => (confirmDelete = true)}>
								Delete User
							</Button>
						{/if}
					</div>
					{#if removeState.error}
						<Alert.Root variant="destructive">
							<AlertCircleIcon />
							<Alert.Title>Failed to delete user</Alert.Title>
							<Alert.Description>{removeState.error}</Alert.Description>
						</Alert.Root>
					{/if}
				{/if}
			</div>
		{/if}
	</Card.Content>
</Card.Root>
