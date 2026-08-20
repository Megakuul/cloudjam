<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import { Separator } from '$lib/components/shad/separator';
	import { DeleteRequestSchema, UpdateRequestSchema } from '$lib/sdk/v1/cloud/provider/provider_pb';
	import { ProviderType, type Provider } from '$lib/sdk/v1/cloud/provider_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import AwsCredentials from '../AwsCredentials.svelte';
	import Spinner from '$lib/components/shad/spinner/spinner.svelte';

	let { provider, refresh, deleted }: { provider: Provider; refresh: () => void; deleted: () => void } = $props();

	// the panel is remounted (keyed) per provider, capturing the initial values is intended.
	// svelte-ignore state_referenced_locally
	let mod = $state({ ...provider });
	// svelte-ignore state_referenced_locally
	let regions = $state(provider.regions.join(', '));
	let confirmDelete = $state(false);

	let updateState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let removeState: SubmitState = $state({ error: '', loading: false, forbidden: false });
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title class="text-2xl">
			{provider.name}
			{#if updateState.loading}
				<Badge>
					<Spinner />
					Reprovisioning
				</Badge>
			{/if}
		</Card.Title>
		<Card.Description>{provider.description}</Card.Description>
		<div class="flex flex-row flex-wrap gap-1">
			<Badge variant="secondary">{ProviderType[provider.type]}</Badge>
			<Badge variant="outline">scope: {provider.scope}</Badge>
			<Badge variant="outline" class="font-mono">{provider.id}</Badge>
		</div>
	</Card.Header>
	<Card.Content class="flex flex-col gap-6">
		<div class="flex flex-col gap-2">
			<Card.Title>Configuration</Card.Title>
			{#if updateState.forbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to update this provider.</p>
			{:else}
				<form
					class="flex flex-col gap-4"
					onsubmit={() =>
						Submit(async () => {
							await Glue.provider.update(
								create(UpdateRequestSchema, {
									mod: {
										...mod,
										regions: regions
											.split(',')
											.map((region) => region.trim())
											.filter((region) => region)
									}
								})
							);
							refresh();
						}, updateState)}
				>
					<div class="grid gap-4 md:grid-cols-2">
						<div class="flex flex-col gap-1">
							<label for="update-name" class="text-sm">Name</label>
							<Input id="update-name" bind:value={mod.name} placeholder="Name of the provider" />
						</div>
						<div class="flex flex-col gap-1">
							<label for="update-description" class="text-sm">Description</label>
							<Input id="update-description" bind:value={mod.description} placeholder="Purpose of the provider" />
						</div>
						<div class="flex flex-col gap-1">
							<label for="update-email" class="text-sm">Email</label>
							<Input
								id="update-email"
								type="email"
								bind:value={mod.email}
								placeholder="Email used to provision new accounts"
							/>
						</div>
						<div class="flex flex-col gap-1">
							<label for="update-regions" class="text-sm">Regions</label>
							<Input id="update-regions" bind:value={regions} placeholder="eu-central-1, us-east-1" />
						</div>
					</div>

					<div class="flex flex-col gap-2">
						<h2 class="text-sm font-medium">Credentials</h2>
						{#if provider.type === ProviderType.AWS}
							<AwsCredentials bind:value={mod.credentials} />
						{:else}
							<Input bind:value={mod.credentials} placeholder="Provider specific credentials" />
						{/if}
					</div>
					<Button type="submit" variant="outline" class="cursor-pointer self-start" disabled={updateState.loading}>
						Update and Reprovision
					</Button>
				</form>
				{#if updateState.error}
					<p class="text-destructive text-xs">{updateState.error}</p>
				{/if}
			{/if}
		</div>

		<Separator />

		<div class="flex flex-col gap-2">
			<Card.Title>Danger Zone</Card.Title>
			{#if removeState.forbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to delete this provider.</p>
			{:else}
				<p class="text-muted-foreground text-sm">
					Before deleting a provider please remove all accounts and challenge definitions.
				</p>
				<div class="flex flex-row items-center gap-2">
					{#if confirmDelete}
						<Button
							variant="destructive"
							class="cursor-pointer"
							disabled={removeState.loading}
							onclick={() =>
								Submit(async () => {
									await Glue.provider.delete(create(DeleteRequestSchema, { id: provider.id }));
									deleted();
								}, removeState)}
						>
							Yes, delete {provider.name}
						</Button>
						<Button variant="outline" class="cursor-pointer" onclick={() => (confirmDelete = false)}>Cancel</Button>
					{:else}
						<Button variant="destructive" class="cursor-pointer" onclick={() => (confirmDelete = true)}>
							Delete Provider
						</Button>
					{/if}
				</div>
				{#if removeState.error}
					<Alert.Root variant="destructive">
						<AlertCircleIcon />
						<Alert.Title>Failed to delete provider</Alert.Title>
						<Alert.Description>{removeState.error}</Alert.Description>
					</Alert.Root>
				{/if}
			{/if}
		</div>
	</Card.Content>
</Card.Root>
