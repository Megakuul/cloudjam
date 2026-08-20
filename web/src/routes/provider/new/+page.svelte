<script lang="ts">
	import { Button } from '$lib/components/shad/button';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import CreateProvider from './CreateProvider.svelte';
</script>

<svelte:head>
	<title>Add Provider | CloudJam</title>
	<meta property="og:title" content="Add Provider | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<Button variant="ghost" size="icon" class="cursor-pointer" href="/provider/">
			<ChevronLeftIcon />
		</Button>
		<h1 class="text-3xl opacity-80">Add Provider</h1>
	</div>

	<CreateProvider />
</div>
