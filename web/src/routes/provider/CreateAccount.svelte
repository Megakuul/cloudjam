<script lang="ts">
	import { Glue, Submit } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import { CreateRequestSchema } from '$lib/sdk/v1/cloud/account/account_pb';
	import { AccountSchema } from '$lib/sdk/v1/cloud/account_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';

	let { providerId, oncreated }: { providerId: string; oncreated: () => void } = $props();

	let error = $state('');
	let loading = $state(false);
	let forbidden = $state(false);

	// the id is generated client side, the scope is inherited from the provider.
	let init = $state(create(AccountSchema, { id: crypto.randomUUID() }));

	let request = $derived(create(CreateRequestSchema, { init: { ...init, providerId: providerId } }));
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title>Provision Account</Card.Title>
		<Card.Description>
			Creates a sandbox account on the provider. Provisioning runs asynchronously; the account joins the pool once it
			reaches the ready state.
		</Card.Description>
	</Card.Header>
	<Card.Content>
		<form
			class="flex flex-col gap-4"
			onsubmit={() =>
				Submit(
					async () => {
						await Glue.account.create(request);
						init = create(AccountSchema, { id: crypto.randomUUID() });
						oncreated();
					},
					(e, l, f) => ((error = e), (loading = l), (forbidden = f))
				)}
		>
			<div class="grid gap-4 md:grid-cols-2">
				<div class="flex flex-col gap-1">
					<label for="create-name" class="text-sm">Name</label>
					<Input id="create-name" bind:value={init.name} placeholder="Name of the account" />
					<p class="text-xs text-destructive {init.name ? '' : 'hidden'}">
						{Glue.Validate(AccountSchema, init).violation.name ?? ''}
					</p>
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-description" class="text-sm">Description</label>
					<Input id="create-description" bind:value={init.description} placeholder="Purpose of the account" />
					<p class="text-destructive text-xs">{Glue.Validate(AccountSchema, init).violation.description ?? ''}</p>
				</div>
			</div>
			<Button
				type="submit"
				class="cursor-pointer self-start"
				disabled={loading || Boolean(Glue.Validate(CreateRequestSchema, request).error)}
			>
				Provision
			</Button>
			{#if error}
				<Alert.Root variant="destructive">
					<AlertCircleIcon />
					<Alert.Title>{forbidden ? 'Permission denied' : 'Failed to provision account'}</Alert.Title>
					<Alert.Description>{error}</Alert.Description>
				</Alert.Root>
			{/if}
		</form>
	</Card.Content>
</Card.Root>
