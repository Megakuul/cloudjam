<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import { compress } from '$lib/compress';
	import * as Alert from '$lib/components/shad/alert';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import { CreateRequestSchema } from '$lib/sdk/v1/cloud/definition/definition_pb';
	import { CompressionMode, DefinitionSchema } from '$lib/sdk/v1/cloud/definition_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import Badge from '$lib/components/shad/badge/badge.svelte';
	import Spinner from '$lib/components/shad/spinner/spinner.svelte';
	import type { Provider } from '$lib/sdk/v1/cloud/provider_pb';
	import LabelInput from '$lib/components/shad/label-input/label-input.svelte';

	let { provider }: { provider: Provider } = $props();

	let init = $derived(create(DefinitionSchema, { id: crypto.randomUUID(), scope: provider.scope }));
	let files: FileList | undefined = $state();

	let createState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let createRequest = $derived(
		create(CreateRequestSchema, { init: { ...init, providerId: provider.id }, compression: CompressionMode.Zstd })
	);
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title>Upload Definition</Card.Title>
		<Card.Description>
			Uploads a compiled challenge plugin to the provider you pick. The wasm binary is zstd compressed in your browser
			before it is sent.
		</Card.Description>
	</Card.Header>
	<Card.Content>
		<form
			class="flex flex-col gap-4"
			onsubmit={() =>
				Submit(async () => {
					const binary = files?.[0] ? await compress(new Uint8Array(await files[0].arrayBuffer())) : new Uint8Array();
					await Glue.definition.create({ ...createRequest, binary: binary });
					init = create(DefinitionSchema, { id: crypto.randomUUID(), scope: provider.scope });
					files = undefined;
				}, createState)}
		>
			<div class="grid gap-4 md:grid-cols-2">
				<LabelInput
					bind:value={init.name}
					label="Name"
					placeholder="Name of the challenge definition"
					validation={Glue.Validate(DefinitionSchema, init).violation.name}
				/>
				<LabelInput
					bind:value={init.description}
					label="Description"
					placeholder="Description of the challenge definition"
					validation={Glue.Validate(DefinitionSchema, init).violation.description}
				/>
				<LabelInput
					bind:value={init.version}
					label="Version"
					placeholder="Plugin Version (currently irrelevant)"
					validation={Glue.Validate(DefinitionSchema, init).violation.version}
				/>
				<div class="flex flex-col gap-1 md:col-span-2">
					<label for="create-binary" class="text-sm">Plugin</label>
					<Input id="create-binary" type="file" accept=".wasm" bind:files />
					<p class="text-muted-foreground text-xs">
						The compiled challenge.wasm (must be smaller than 50 MB after zstd compression)
					</p>
				</div>
			</div>
			<div class="flex flex-row items-center justify-start gap-2">
				<Button
					type="submit"
					class="cursor-pointer self-start"
					disabled={createState.loading ||
						!files?.length ||
						Boolean(Glue.Validate(CreateRequestSchema, createRequest).error)}
				>
					Upload
				</Button>
				{#if createState.loading}
					<Badge>
						<Spinner />
						Compressing and Uploading
					</Badge>
				{/if}
			</div>

			{#if createState.error}
				<Alert.Root variant="destructive">
					<AlertCircleIcon />
					<Alert.Title>{createState.forbidden ? 'Permission denied' : 'Failed to upload definition'}</Alert.Title>
					<Alert.Description>{createState.error}</Alert.Description>
				</Alert.Root>
			{/if}
		</form>
	</Card.Content>
</Card.Root>
