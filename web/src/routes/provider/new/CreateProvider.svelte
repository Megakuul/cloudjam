<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
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
	import OptionalSelect from '$lib/components/custom/OptionalSelect.svelte';
	import { scopes } from '$lib/scopes.svelte';
	import { goto } from '$app/navigation';
	import LabelInput from '$lib/components/shad/label-input/label-input.svelte';

	const types = [{ value: String(ProviderType.AWS), label: 'AWS' }];

	let init = $state(create(ProviderSchema, { id: crypto.randomUUID() }));
	let type = $state(String(ProviderType.AWS));
	let regions = $state('');

	let createState: SubmitState = $state({ error: '', loading: false, forbidden: false });

	let createRequest = $derived(
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
			Registers a CloudJam cloud provider. Depending on provider this may already provision some bootstrap
			infrastructure.
		</Card.Description>
	</Card.Header>
	<Card.Content>
		<form
			class="flex flex-col gap-4"
			onsubmit={() =>
				Submit(async () => {
					const resp = await Glue.provider.create(createRequest);
					goto(`/provider/${resp.id}`);
				}, createState)}
		>
			<div class="grid gap-4 md:grid-cols-2">
				<LabelInput
					bind:value={init.name}
					label="Name"
					placeholder="Name of the provider"
					validation={Glue.Validate(ProviderSchema, init).violation.name}
				/>
				<div class="flex flex-col gap-1">
					<label class="text-sm">
						Type
						<Select.Root type="single" bind:value={type}>
							<Select.Trigger class="w-full cursor-pointer">
								{types.find((t) => t.value === type)?.label}
							</Select.Trigger>
							<Select.Content>
								{#each types as item (item.value)}
									<Select.Item value={item.value} label={item.label}>{item.label}</Select.Item>
								{/each}
							</Select.Content>
						</Select.Root>
					</label>
				</div>
				<LabelInput
					bind:value={init.description}
					label="Description"
					placeholder="Description of the provider"
					validation={Glue.Validate(ProviderSchema, init).violation.description}
				/>
				<LabelInput
					bind:value={init.email}
					label="Email"
					placeholder="Root email of the provider (and prefix for accounts)"
					validation={Glue.Validate(ProviderSchema, init).violation.email}
				/>
				<LabelInput
					bind:value={regions}
					label="Regions"
					placeholder="Comma separated list of supported regions (e.g. us-east-1, eu-central-1, ...)"
					validation={''}
				/>
				<div class="flex flex-col gap-1">
					<label class="text-sm">
						Scope
						<OptionalSelect
							bind:value={init.scope}
							placeholder="Scope the provider is placed in"
							suggestions={scopes.map((scope) => ({ id: scope, title: scope }))}
						/>
					</label>
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
				disabled={createState.loading || Boolean(Glue.Validate(CreateRequestSchema, createRequest).error)}
			>
				Add
			</Button>
			{#if createState.error}
				<Alert.Root variant="destructive">
					<AlertCircleIcon />
					<Alert.Title>{createState.forbidden ? 'Permission denied' : 'Failed to add provider'}</Alert.Title>
					<Alert.Description>{createState.error}</Alert.Description>
				</Alert.Root>
			{/if}
		</form>
	</Card.Content>
</Card.Root>
