<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import OptionalSelect from '$lib/components/custom/OptionalSelect.svelte';
	import * as Alert from '$lib/components/shad/alert';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import LabelInput from '$lib/components/shad/label-input/label-input.svelte';
	import * as Select from '$lib/components/shad/select';
	import { CreateRequestSchema } from '$lib/sdk/v1/admin/user/user_pb';
	import { UserSchema } from '$lib/sdk/v1/admin/user_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampFromDate } from '@bufbuild/protobuf/wkt';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import CopyIcon from '@lucide/svelte/icons/copy';

	let { scopes = [], oncreated }: { scopes?: string[]; oncreated: () => void } = $props();

	const expiries = [
		{ value: '1', label: '1 hour' },
		{ value: '24', label: '1 day' },
		{ value: '168', label: '7 days' }
	];

	let createState: SubmitState = $state({ error: '', forbidden: false, loading: false });

	let init = $state(create(UserSchema, {}));
	let expiry = $state('24');
	let code = $state('');
	let link = $state('');

	let request = $derived(
		create(CreateRequestSchema, {
			init: init,
			expires: timestampFromDate(new Date(Date.now() + Number(expiry) * 60 * 60 * 1000))
		})
	);
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title>Invite User</Card.Title>
		<Card.Description>
			Creates a new user and returns a one time invitation code to complete the registration.
		</Card.Description>
	</Card.Header>
	<Card.Content>
		{#if code}
			<Alert.Root>
				<Alert.Title>Invitation created</Alert.Title>
				<Alert.Description class="flex flex-col gap-2">
					<span>Share the registration link (or the raw code) with the invited user:</span>
					<span class="flex flex-row items-center gap-2 font-mono text-xs break-all">
						{code}
						<Button
							variant="outline"
							size="icon"
							title="Copy code"
							class="cursor-pointer"
							onclick={() => navigator.clipboard.writeText(code)}
						>
							<CopyIcon />
						</Button>
					</span>
					<span class="flex flex-row items-center gap-2 font-mono text-xs break-all">
						{link}
						<Button
							variant="outline"
							size="icon"
							title="Copy link"
							class="cursor-pointer"
							onclick={() => navigator.clipboard.writeText(link)}
						>
							<CopyIcon />
						</Button>
					</span>
				</Alert.Description>
			</Alert.Root>
		{:else}
			<form
				class="flex flex-col gap-4"
				onsubmit={() =>
					Submit(async () => {
						const resp = await Glue.user.create(request);
						code = resp.code;
						link = `${location.origin}/register?email=${encodeURIComponent(init.email)}&username=${encodeURIComponent(init.username)}&code=${encodeURIComponent(resp.code)}`;
						oncreated();
					}, createState)}
			>
				<div class="grid gap-4 md:grid-cols-2">
					<LabelInput
						bind:value={init.username}
						label="Username"
						placeholder="Username of the new user"
						validation={Glue.Validate(UserSchema, init).violation.username}
					/>
					<LabelInput
						bind:value={init.email}
						label="Email"
						placeholder="Email of the new user"
						validation={Glue.Validate(UserSchema, init).violation.email}
					/>
					<LabelInput
						bind:value={init.organization}
						label="Organization"
						placeholder="Organization of the new user"
						validation={Glue.Validate(UserSchema, init).violation.organization}
					/>
					<LabelInput
						bind:value={init.description}
						label="Slogan"
						placeholder="Slogan for the new user"
						validation={Glue.Validate(UserSchema, init).violation.description}
					/>
					<label class="flex flex-col gap-1 text-sm">
						Scope
						<OptionalSelect
							bind:value={init.scope}
							placeholder="Scope the user is placed in"
							suggestions={scopes.map((scope) => ({ id: scope, title: scope }))}
						/>
					</label>
					<div class="flex flex-col gap-1">
						<label for="create-expiry" class="text-sm">Invitation expires in</label>
						<Select.Root type="single" bind:value={expiry}>
							<Select.Trigger id="create-expiry" class="w-full cursor-pointer">
								{expiries.find((e) => e.value === expiry)?.label}
							</Select.Trigger>
							<Select.Content>
								{#each expiries as item (item.value)}
									<Select.Item value={item.value} label={item.label}>{item.label}</Select.Item>
								{/each}
							</Select.Content>
						</Select.Root>
					</div>
				</div>
				<Button
					type="submit"
					class="cursor-pointer self-start"
					disabled={createState.loading || Boolean(Glue.Validate(CreateRequestSchema, request).error)}
				>
					Invite
				</Button>
				{#if createState.error}
					<Alert.Root variant="destructive">
						<AlertCircleIcon />
						<Alert.Title>{createState.forbidden ? 'Permission denied' : 'Failed to invite user'}</Alert.Title>
						<Alert.Description>{createState.error}</Alert.Description>
					</Alert.Root>
				{/if}
			</form>
		{/if}
	</Card.Content>
</Card.Root>
