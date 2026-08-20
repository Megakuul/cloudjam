<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { ConfigureRoleRequestSchema } from '$lib/sdk/v1/admin/rbac/rbac_pb';
	import { CreateRequestSchema } from '$lib/sdk/v1/admin/role/role_pb';
	import { RoleSchema } from '$lib/sdk/v1/admin/role_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import PermissionEditor from '../PermissionEditor.svelte';
	import { scopes } from '$lib/scopes.svelte';
	import OptionalSelect from '$lib/components/custom/OptionalSelect.svelte';
	import LabelInput from '$lib/components/shad/label-input/label-input.svelte';

	let { oncreated }: { oncreated: () => void } = $props();

	let configureState: SubmitState = $state({ error: '', loading: false, forbidden: false });

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
		<form
			class="flex flex-col gap-4"
			onsubmit={() =>
				Submit(async () => {
					await Glue.role.create(create(CreateRequestSchema, { init: init }));
					// create only persists the metadata, the permission map
					// is configured over the dedicated rbac endpoint.
					await Glue.rbac.configureRole(create(ConfigureRoleRequestSchema, { mod: init }));
					id = crypto.randomUUID();
					name = description = scope = '';
					entries = [{ scope: '', patterns: '' }];
					oncreated();
				}, configureState)}
		>
			<div class="grid gap-4 md:grid-cols-2">
				<LabelInput
					bind:value={name}
					label="Name"
					placeholder="Name of the new role"
					validation={Glue.Validate(RoleSchema, init).violation.name}
				/>
				<LabelInput
					bind:value={description}
					label="Description"
					placeholder="Description of the new role"
					validation={Glue.Validate(RoleSchema, init).violation.description}
				/>
				<div class="flex flex-col gap-1">
					<label class="text-sm">
						Scope
						<OptionalSelect
							bind:value={scope}
							placeholder="Scope the role is placed in"
							suggestions={scopes.map((scope) => ({ id: scope, title: scope }))}
						/>
					</label>
				</div>
			</div>
			<div class="flex flex-col gap-1">
				<span class="text-sm">Permissions</span>
				<PermissionEditor bind:entries />
			</div>
			<Button
				type="submit"
				class="cursor-pointer self-start"
				disabled={configureState.loading || Boolean(Glue.Validate(RoleSchema, init).error)}
			>
				Create
			</Button>
			{#if configureState.error}
				<Alert.Root variant="destructive">
					<AlertCircleIcon />
					<Alert.Title>{configureState.forbidden ? 'Permission denied' : 'Failed to create role'}</Alert.Title>
					<Alert.Description>{configureState.error}</Alert.Description>
				</Alert.Root>
			{/if}
		</form>
	</Card.Content>
</Card.Root>
