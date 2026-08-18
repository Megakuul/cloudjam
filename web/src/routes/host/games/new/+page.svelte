<script lang="ts">
	import { goto } from '$app/navigation';
	import { Button } from '$lib/components/shad/button';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import CreateGame from './CreateGame.svelte';
</script>

<svelte:head>
	<title>Host Game | CloudJam</title>
	<meta property="og:title" content="Host Game | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<Button variant="ghost" size="icon" class="cursor-pointer" href="/host/games/">
			<ChevronLeftIcon />
		</Button>
		<h1 class="text-3xl opacity-80">Host Game</h1>
	</div>

	<CreateGame oncreated={(id) => goto(`/host/games/${id}`)} />
</div>
