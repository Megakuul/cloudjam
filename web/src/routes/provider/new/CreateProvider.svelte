<script lang="ts">
	import { Glue, Submit } from '$lib';
	import ScopeInput from '$lib/components/custom/ScopeInput.svelte';
	import * as Alert from '$lib/components/shad/alert';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import * as Select from '$lib/components/shad/select';
	import { CreateRequestSchema } from '$lib/sdk/v1/cloud/provider/provider_pb';
	import { ProviderSchema, ProviderType } from '$lib/sdk/v1/cloud/provider_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import AwsCredentials from '../AwsCredentials.svelte';

	let { scopes = [], oncreated }: { scopes?: string[]; oncreated: (id: string) => void } = $props();

	const types = [{ value: String(ProviderType.AWS), label: 'AWS' }];

	let error = $state('');
	let loading = $state(false);
	let forbidden = $state(false);

	// the id is generated client side, the server stores whatever it is given.
	let init = $state(create(ProviderSchema, { id: crypto.randomUUID() }));
	let type = $state(String(ProviderType.AWS));
	let regions = $state('');

	let request = $derived(
		create(CreateRequestSchema, {
			init: {
				...init,
				type: Number(type) as ProviderType,
				regions: regions
					.split(',')
					.map((region) => region.trim())
					.filter((region) => region)
			}
		})
	);
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title>Add Provider</Card.Title>
		<Card.Description>
			Registers a cloud provider account. CloudJam provisions the sandbox accounts of the pool inside it.
		</Card.Description>
	</Card.Header>
	<Card.Content>
		<form
			class="flex flex-col gap-4"
			onsubmit={() =>
				Submit(
					async () => {
						await Glue.provider.create(request);
						// the new provider is opened right away, nobody has to scan for it.
						oncreated(init.id);
						init = create(ProviderSchema, { id: crypto.randomUUID() });
						regions = '';
					},
					(e, l, f) => ((error = e), (loading = l), (forbidden = f))
				)}
		>
			<div class="grid gap-4 md:grid-cols-2">
				<div class="flex flex-col gap-1">
					<label for="create-name" class="text-sm">Name</label>
					<Input id="create-name" bind:value={init.name} placeholder="Name of the provider" />
					<p class="text-destructive text-xs">{Glue.Validate(ProviderSchema, init).violation.name ?? ''}</p>
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-type" class="text-sm">Type</label>
					<Select.Root type="single" bind:value={type}>
						<Select.Trigger id="create-type" class="w-full cursor-pointer">
							{types.find((t) => t.value === type)?.label}
						</Select.Trigger>
						<Select.Content>
							{#each types as item (item.value)}
								<Select.Item value={item.value} label={item.label}>{item.label}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-description" class="text-sm">Description</label>
					<Input id="create-description" bind:value={init.description} placeholder="Purpose of the provider" />
					<p class="text-destructive text-xs">{Glue.Validate(ProviderSchema, init).violation.description ?? ''}</p>
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-email" class="text-sm">Email</label>
					<Input id="create-email" type="email" bind:value={init.email} placeholder="Root email of the provider" />
					<p class="text-destructive text-xs">{Glue.Validate(ProviderSchema, init).violation.email ?? ''}</p>
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-regions" class="text-sm">Regions</label>
					<Input id="create-regions" bind:value={regions} placeholder="eu-central-1, us-east-1" />
					<p class="text-muted-foreground text-xs">Comma separated list of regions accounts are placed in.</p>
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-scope" class="text-sm">Scope</label>
					<ScopeInput
						id="create-scope"
						bind:value={init.scope}
						{scopes}
						placeholder="Scope the provider is placed in"
					/>
					<p class="text-muted-foreground text-xs">You can only attach a scope you possess yourself.</p>
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<h2 class="text-sm font-medium">Credentials</h2>
				{#if Number(type) === ProviderType.AWS}
					<AwsCredentials bind:value={init.credentials} />
				{:else}
					<Input bind:value={init.credentials} placeholder="Provider specific credentials" />
				{/if}
				<p class="text-destructive text-xs">{Glue.Validate(ProviderSchema, init).violation.credentials ?? ''}</p>
			</div>
			<Button
				type="submit"
				class="cursor-pointer self-start"
				disabled={loading || Boolean(Glue.Validate(CreateRequestSchema, request).error)}
			>
				Add
			</Button>
			{#if error}
				<Alert.Root variant="destructive">
					<AlertCircleIcon />
					<Alert.Title>{forbidden ? 'Permission denied' : 'Failed to add provider'}</Alert.Title>
					<Alert.Description>{error}</Alert.Description>
				</Alert.Root>
			{/if}
		</form>
	</Card.Content>
</Card.Root>
