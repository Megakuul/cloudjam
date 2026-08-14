<script lang="ts">
	import { Glue, Submit } from '$lib';
	import { compress } from '$lib/compress';
	import ProviderSelect from '$lib/components/custom/ProviderSelect.svelte';
	import ScopeInput from '$lib/components/custom/ScopeInput.svelte';
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

	let {
		providerId = $bindable(),
		scope,
		onprovider,
		oncreated
	}: { providerId: string; scope: string; onprovider: () => void; oncreated: () => void } = $props();

	let error = $state('');
	let loading = $state(false);
	let forbidden = $state(false);

	// the id is generated client side, the server stores whatever it is given.
	// svelte-ignore state_referenced_locally
	let init = $state(create(DefinitionSchema, { id: crypto.randomUUID(), scope: scope }));
	let files: FileList | undefined = $state();

	// picking a provider re-defaults the scope of the definition to the provider's scope.
	$effect(() => {
		init.scope = scope;
	});

	let request = $derived(
		create(CreateRequestSchema, { init: { ...init, providerId: providerId }, compression: CompressionMode.Zstd })
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
				Submit(
					async () => {
						const binary = files?.[0] ? await compress(new Uint8Array(await files[0].arrayBuffer())) : new Uint8Array();
						await Glue.definition.create({ ...request, binary: binary });
						init = create(DefinitionSchema, { id: crypto.randomUUID(), scope: scope });
						files = undefined;
						oncreated();
					},
					(e, l, f) => ((error = e), (loading = l), (forbidden = f))
				)}
		>
			<div class="flex flex-col gap-1">
				<label for="create-provider" class="text-sm">Provider</label>
				<ProviderSelect bind:value={providerId} onselect={() => onprovider()} />
				<p class="text-muted-foreground text-xs">The provider the plugin is stored on and later run by.</p>
			</div>

			<div class="grid gap-4 md:grid-cols-2">
				<div class="flex flex-col gap-1">
					<label for="create-name" class="text-sm">Name</label>
					<Input id="create-name" bind:value={init.name} placeholder="Name of the definition" />
					<p class="text-destructive text-xs">{Glue.Validate(DefinitionSchema, init).violation.name ?? ''}</p>
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-version" class="text-sm">Version</label>
					<Input id="create-version" bind:value={init.version} placeholder="1.0.0" />
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-description" class="text-sm">Description</label>
					<Input id="create-description" bind:value={init.description} placeholder="What the challenge is about" />
					<p class="text-destructive text-xs">{Glue.Validate(DefinitionSchema, init).violation.description ?? ''}</p>
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-scope" class="text-sm">Scope</label>
					<ScopeInput
						id="create-scope"
						bind:value={init.scope}
						scopes={[scope]}
						placeholder="Scope the definition is placed in"
					/>
					<p class="text-muted-foreground text-xs">Defaults to the scope of the provider.</p>
				</div>
				<div class="flex flex-col gap-1 md:col-span-2">
					<label for="create-binary" class="text-sm">Plugin</label>
					<Input id="create-binary" type="file" accept=".wasm" bind:files />
					<p class="text-muted-foreground text-xs">The compiled challenge.wasm, at most 50 MB.</p>
				</div>
			</div>
			<div class="flex flex-row items-center justify-start gap-2">
				<Button
					type="submit"
					class="cursor-pointer self-start"
					disabled={loading ||
						!providerId ||
						!files?.length ||
						Boolean(Glue.Validate(CreateRequestSchema, request).error)}
				>
					Upload
				</Button>
				{#if loading}
					<Badge>
						<Spinner />
						Compressing and Uploading
					</Badge>
				{/if}
			</div>

			{#if error}
				<Alert.Root variant="destructive">
					<AlertCircleIcon />
					<Alert.Title>{forbidden ? 'Permission denied' : 'Failed to upload definition'}</Alert.Title>
					<Alert.Description>{error}</Alert.Description>
				</Alert.Root>
			{/if}
		</form>
	</Card.Content>
</Card.Root>
