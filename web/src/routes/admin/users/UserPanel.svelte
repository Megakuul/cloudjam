<script lang="ts">
	import { Glue, Submit } from '$lib';
	import ScopeInput from '$lib/components/custom/ScopeInput.svelte';
	import * as Alert from '$lib/components/ui/alert';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import * as Select from '$lib/components/ui/select';
	import { Separator } from '$lib/components/ui/separator';
	import { AttachRoleRequestSchema, AttachScopeRequestSchema, Resource } from '$lib/sdk/v1/admin/rbac/rbac_pb';
	import type { Role } from '$lib/sdk/v1/admin/role_pb';
	import {
		DeleteRequestSchema,
		GetRequestSchema,
		ResetPasswordRequestSchema,
		UpdateRequestSchema
	} from '$lib/sdk/v1/admin/user/user_pb';
	import type { User } from '$lib/sdk/v1/admin/user_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampFromDate } from '@bufbuild/protobuf/wkt';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import { toSvg } from 'jdenticon';
	import { onMount } from 'svelte';
	import { Icon } from 'svelte-ux';

	let {
		user,
		roles,
		rolesForbidden,
		scopes,
		refresh
	}: { user: User; roles: Role[]; rolesForbidden: boolean; scopes: string[]; refresh: () => void } = $props();

	type section = { error: string; loading: boolean; forbidden: boolean };
	const setter = (s: section) => (e: string, l: boolean, f: boolean) => (
		(s.error = e),
		(s.loading = l),
		(s.forbidden = f)
	);

	let scope = $state(''); // scope is not part of the list output, it is enriched by a get request.
	// the panel is remounted (keyed) per user, capturing the initial value is intended.
	// svelte-ignore state_referenced_locally
	let organization = $state(user.organization);
	// svelte-ignore state_referenced_locally
	let role = $state(user.role);
	let attachScope = $state('');
	let attachResource = $state('user');
	let resetCode = $state('');
	let confirmDelete = $state(false);

	let update: section = $state({ error: '', loading: false, forbidden: false });
	let attachRole: section = $state({ error: '', loading: false, forbidden: false });
	let attach: section = $state({ error: '', loading: false, forbidden: false });
	let reset: section = $state({ error: '', loading: false, forbidden: false });
	let remove: section = $state({ error: '', loading: false, forbidden: false });

	const resources = [
		{ value: 'user', label: 'User data' },
		{ value: 'creds', label: 'Login credentials' }
	];

	onMount(() => {
		Submit(
			async () => {
				scope = (await Glue.user.get(create(GetRequestSchema, { id: user.id }))).user?.scope ?? '';
			},
			() => {}
		);
	});
</script>

<Card.Root class="w-full">
	<Card.Header class="flex flex-row gap-4 items-center">
		<Icon svg={toSvg(user.pubId, 20)} width="4rem" height="4rem" class="rounded-lg bg-primary/5" />
		<div class="flex flex-col gap-1">
			<Card.Title class="text-2xl">{user.username}</Card.Title>
			<Card.Description>{user.email}</Card.Description>
			<div class="flex flex-row flex-wrap gap-1">
				{#if user.privileged}
					<Badge variant="secondary">privileged</Badge>
				{/if}
				{#if scope}
					<Badge variant="outline">scope: {scope}</Badge>
				{/if}
				{#if user.role}
					<Badge variant="outline">role: {roles.find((r) => r.id === user.role)?.name ?? user.role}</Badge>
				{/if}
			</div>
		</div>
	</Card.Header>
	<Card.Content class="flex flex-col gap-6">
		{#if user.privileged}
			<p class="text-sm italic text-muted-foreground">
				This user is privileged; role, scope and profile cannot be modified.
			</p>
		{:else}
			<div class="flex flex-col gap-2">
				<Card.Title>Organization</Card.Title>
				{#if update.forbidden}
					<p class="text-sm italic text-muted-foreground">You are not allowed to update this user.</p>
				{:else}
					<form
						class="flex flex-row gap-2 items-center"
						onsubmit={() =>
							Submit(async () => {
								await Glue.user.update(create(UpdateRequestSchema, { mod: { ...user, organization: organization } }));
								refresh();
							}, setter(update))}
					>
						<Input class="max-w-96" bind:value={organization} placeholder="Organization of the user" />
						<Button type="submit" variant="outline" class="cursor-pointer" disabled={update.loading}>Save</Button>
					</form>
					{#if update.error}
						<p class="text-xs text-destructive">{update.error}</p>
					{/if}
				{/if}
			</div>

			<Separator />

			<div class="flex flex-col gap-2">
				<Card.Title>Role</Card.Title>
				{#if rolesForbidden || attachRole.forbidden}
					<p class="text-sm italic text-muted-foreground">You are not allowed to attach roles.</p>
				{:else}
					<div class="flex flex-row gap-2 items-center">
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
							disabled={attachRole.loading || !role || role === user.role}
							onclick={() =>
								Submit(async () => {
									await Glue.rbac.attachRole(create(AttachRoleRequestSchema, { id: user.id, role: role }));
									refresh();
								}, setter(attachRole))}
						>
							Attach
						</Button>
					</div>
					{#if attachRole.error}
						<p class="text-xs text-destructive">{attachRole.error}</p>
					{/if}
				{/if}
			</div>

			<Separator />

			<div class="flex flex-col gap-2">
				<Card.Title>Scope</Card.Title>
				{#if attach.forbidden}
					<p class="text-sm italic text-muted-foreground">You are not allowed to attach scopes.</p>
				{:else}
					<p class="text-sm text-muted-foreground">
						Moves the selected resource of this user into another scope you possess. User data is keyed by the user id,
						login credentials by the email.
					</p>
					<div class="flex flex-row gap-2 items-center">
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
						<ScopeInput class="max-w-48" bind:value={attachScope} {scopes} placeholder="New scope" />
						<Button
							variant="outline"
							class="cursor-pointer"
							disabled={attach.loading || !attachScope}
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

			<Separator />
		{/if}

		<div class="flex flex-col gap-2">
			<Card.Title>Reset Password</Card.Title>
			{#if reset.forbidden}
				<p class="text-sm italic text-muted-foreground">You are not allowed to reset passwords.</p>
			{:else if resetCode}
				<Alert.Root>
					<Alert.Title>Reset code created</Alert.Title>
					<Alert.Description class="flex flex-row gap-2 items-center font-mono text-xs break-all">
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
				<div class="flex flex-row gap-2 items-center">
					<Button
						variant="outline"
						class="cursor-pointer"
						disabled={reset.loading}
						onclick={() =>
							Submit(async () => {
								const resp = await Glue.user.resetPassword(
									create(ResetPasswordRequestSchema, {
										email: user.email,
										expires: timestampFromDate(new Date(Date.now() + 24 * 60 * 60 * 1000)) // expires in 24 hours
									})
								);
								resetCode = resp.code;
							}, setter(reset))}
					>
						Create Reset Code
					</Button>
					<p class="text-sm text-muted-foreground">
						Creates a one time code (valid 24 hours) to redo the registration.
					</p>
				</div>
				{#if reset.error}
					<p class="text-xs text-destructive">{reset.error}</p>
				{/if}
			{/if}
		</div>

		{#if !user.privileged}
			<Separator />

			<div class="flex flex-col gap-2">
				<Card.Title>Danger Zone</Card.Title>
				{#if remove.forbidden}
					<p class="text-sm italic text-muted-foreground">You are not allowed to delete this user.</p>
				{:else}
					<div class="flex flex-row gap-2 items-center">
						{#if confirmDelete}
							<Button
								variant="destructive"
								class="cursor-pointer"
								disabled={remove.loading}
								onclick={() =>
									Submit(async () => {
										await Glue.user.delete(create(DeleteRequestSchema, { id: user.id }));
										refresh();
									}, setter(remove))}
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
					{#if remove.error}
						<Alert.Root variant="destructive">
							<AlertCircleIcon />
							<Alert.Title>Failed to delete user</Alert.Title>
							<Alert.Description>{remove.error}</Alert.Description>
						</Alert.Root>
					{/if}
				{/if}
			</div>
		{/if}
	</Card.Content>
</Card.Root>
