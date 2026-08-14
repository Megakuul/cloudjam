<script lang="ts">
	import { Glue, Submit } from '$lib';
	import { compress } from '$lib/compress';
	import { toDigest } from '$lib/digest';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import { Separator } from '$lib/components/shad/separator';
	import { DeleteRequestSchema, UpdateRequestSchema } from '$lib/sdk/v1/cloud/definition/definition_pb';
	import { CompressionMode, type Definition } from '$lib/sdk/v1/cloud/definition_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import Spinner from '$lib/components/shad/spinner/spinner.svelte';

	let { definition, refresh }: { definition: Definition; refresh: () => void } = $props();

	type section = { error: string; loading: boolean; forbidden: boolean };
	const setter = (s: section) => (e: string, l: boolean, f: boolean) => (
		(s.error = e),
		(s.loading = l),
		(s.forbidden = f)
	);

	// the panel is remounted (keyed) per definition, capturing the initial values is intended.
	// svelte-ignore state_referenced_locally
	let mod = $state({ ...definition });
	let files: FileList | undefined = $state();
	let confirmDelete = $state(false);

	let update: section = $state({ error: '', loading: false, forbidden: false });
	let remove: section = $state({ error: '', loading: false, forbidden: false });
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title class="text-2xl">{definition.name}</Card.Title>
		<Card.Description>{definition.description}</Card.Description>
		<div class="flex flex-row flex-wrap gap-1">
			<Badge variant="secondary">version: {definition.version || 'unversioned'}</Badge>
			<Badge variant="outline">scope: {definition.scope}</Badge>
			<Badge variant="outline" class="font-mono">{definition.id}</Badge>
			{#if definition.hash}
				<Badge variant="outline" class="font-mono">{toDigest(definition.hash)}</Badge>
			{/if}
		</div>
	</Card.Header>
	<Card.Content class="flex flex-col gap-6">
		<div class="flex flex-col gap-2">
			<Card.Title>Definition</Card.Title>
			{#if update.forbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to update this definition.</p>
			{:else}
				<form
					class="flex flex-col gap-4"
					onsubmit={() =>
						Submit(async () => {
							const binary = files?.[0]
								? await compress(new Uint8Array(await files[0].arrayBuffer()))
								: new Uint8Array();
							await Glue.definition.update({
								...create(UpdateRequestSchema, { mod: mod, compression: CompressionMode.Zstd }),
								binary: binary
							});
							files = undefined;
							refresh();
						}, setter(update))}
				>
					<div class="grid gap-4 md:grid-cols-2">
						<div class="flex flex-col gap-1">
							<label for="update-name" class="text-sm">Name</label>
							<Input id="update-name" bind:value={mod.name} placeholder="Name of the definition" />
						</div>
						<div class="flex flex-col gap-1">
							<label for="update-version" class="text-sm">Version</label>
							<Input id="update-version" bind:value={mod.version} placeholder="1.0.0" />
						</div>
						<div class="flex flex-col gap-1">
							<label for="update-description" class="text-sm">Description</label>
							<Input id="update-description" bind:value={mod.description} placeholder="What the challenge is about" />
						</div>
						<div class="flex flex-col gap-1">
							<label for="update-binary" class="text-sm">Plugin</label>
							<Input id="update-binary" type="file" accept=".wasm" bind:files />
							<p class="text-muted-foreground text-xs">Optional; the stored plugin is kept if no file is selected.</p>
						</div>
					</div>

					<div class="flex flex-row items-center justify-start gap-2">
						<Button type="submit" variant="outline" class="cursor-pointer self-start" disabled={update.loading}>
							Save and Upload
						</Button>
						{#if update.loading && files}
							<Badge>
								<Spinner />
								Compressing and Uploading
							</Badge>
						{/if}
					</div>
				</form>
				{#if update.error}
					<p class="text-destructive text-xs">{update.error}</p>
				{/if}
			{/if}
		</div>

		<Separator />

		<div class="flex flex-col gap-2">
			<Card.Title>Danger Zone</Card.Title>
			{#if remove.forbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to delete this definition.</p>
			{:else}
				<div class="flex flex-row items-center gap-2">
					{#if confirmDelete}
						<Button
							variant="destructive"
							class="cursor-pointer"
							disabled={remove.loading}
							onclick={() =>
								Submit(async () => {
									await Glue.definition.delete(
										create(DeleteRequestSchema, { providerId: definition.providerId, id: definition.id })
									);
									refresh();
								}, setter(remove))}
						>
							Yes, delete {definition.name}
						</Button>
						<Button variant="outline" class="cursor-pointer" onclick={() => (confirmDelete = false)}>Cancel</Button>
					{:else}
						<Button variant="destructive" class="cursor-pointer" onclick={() => (confirmDelete = true)}>
							Delete Definition
						</Button>
					{/if}
				</div>
				{#if remove.error}
					<Alert.Root variant="destructive">
						<AlertCircleIcon />
						<Alert.Title>Failed to delete definition</Alert.Title>
						<Alert.Description>{remove.error}</Alert.Description>
					</Alert.Root>
				{/if}
			{/if}
		</div>
	</Card.Content>
</Card.Root>
