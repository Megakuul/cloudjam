<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import { compress } from '$lib/compress';
	import { toDigest } from '$lib/digest';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import { Separator } from '$lib/components/shad/separator';
	import { DeleteRequestSchema, UpdateRequestSchema } from '$lib/sdk/v1/cloud/definition/definition_pb';
	import { CompressionMode, DefinitionSchema, type Definition } from '$lib/sdk/v1/cloud/definition_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import Spinner from '$lib/components/shad/spinner/spinner.svelte';
	import LabelInput from '$lib/components/shad/label-input/label-input.svelte';

	let { definition, refresh, close }: { definition: Definition; refresh: () => void; close: () => void } = $props();

	let mod = $derived({ ...definition });
	let files: FileList | undefined = $state();
	let confirmDelete = $state(false);

	let updateState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let removeState: SubmitState = $state({ error: '', loading: false, forbidden: false });
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
			{#if updateState.forbidden}
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
						}, updateState)}
				>
					<div class="grid gap-4 md:grid-cols-2">
						<LabelInput
							bind:value={mod.name}
							label="Name"
							placeholder="Name of the challenge definition"
							validation={Glue.Validate(DefinitionSchema, mod).violation.name}
						/>
						<LabelInput
							bind:value={mod.description}
							label="Description"
							placeholder="Description of the challenge definition"
							validation={Glue.Validate(DefinitionSchema, mod).violation.description}
						/>
						<LabelInput
							bind:value={mod.version}
							label="Version"
							placeholder="Plugin Version (currently irrelevant)"
							validation={Glue.Validate(DefinitionSchema, mod).violation.version}
						/>
						<div class="flex flex-col gap-1">
							<label for="update-binary" class="text-sm">Plugin</label>
							<Input id="update-binary" type="file" accept=".wasm" bind:files />
							<p class="text-muted-foreground text-xs">Optional; the stored plugin is kept if no file is selected.</p>
						</div>
					</div>

					<div class="flex flex-row items-center justify-start gap-2">
						<Button type="submit" variant="outline" class="cursor-pointer self-start" disabled={updateState.loading}>
							Save and Upload
						</Button>
						{#if updateState.loading && files}
							<Badge>
								<Spinner />
								Compressing and Uploading
							</Badge>
						{/if}
					</div>
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
				<p class="text-muted-foreground text-sm italic">You are not allowed to delete this definition.</p>
			{:else}
				<div class="flex flex-row items-center gap-2">
					{#if confirmDelete}
						<Button
							variant="destructive"
							class="cursor-pointer"
							disabled={removeState.loading}
							onclick={() =>
								Submit(async () => {
									await Glue.definition.delete(
										create(DeleteRequestSchema, { providerId: definition.providerId, id: definition.id })
									);
									refresh();
									close();
								}, removeState)}
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
				{#if removeState.error}
					<Alert.Root variant="destructive">
						<AlertCircleIcon />
						<Alert.Title>Failed to delete definition</Alert.Title>
						<Alert.Description>{removeState.error}</Alert.Description>
					</Alert.Root>
				{/if}
			{/if}
		</div>
	</Card.Content>
</Card.Root>
