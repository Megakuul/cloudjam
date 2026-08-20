<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import { Button } from '$lib/components/shad/button';
	import { GetRequestSchema } from '$lib/sdk/v1/cloud/provider/provider_pb';
	import { create } from '@bufbuild/protobuf';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import CreateDefinition from './CreateDefinition.svelte';
	import DefinitionSection from './DefinitionSection.svelte';
</script>

<svelte:head>
	<title>Design | CloudJam</title>
	<meta property="og:title" content="Design | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<Button variant="ghost" size="icon" class="cursor-pointer" href="/host/">
			<ChevronLeftIcon />
		</Button>
		<h1 class="text-3xl opacity-80">Design</h1>
	</div>

	<p class="text-muted-foreground text-sm">
		Challenge plugins are compiled to wasm and stored on the provider that runs them. A challenge handed out in a game
		references one of them.
	</p>

	<CreateDefinition bind:providerId onprovider={() => select()} />

	{#if providerId}
		<DefinitionSection {providerId} />
	{/if}
</div>
