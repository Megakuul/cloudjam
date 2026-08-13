<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import GameList from './GameList.svelte';
	import GameWorkspace from './GameWorkspace.svelte';

	// a game is a root object; teams and challenges live inside it and are only reachable
	// through it. the id in the url keeps the workspace deep linkable, so nobody has to scan
	// the game table to get back to a game they already know.
	let gameId = $state('');

	function open(id: string) {
		gameId = id;
		goto(id ? `/host/games/?id=${encodeURIComponent(id)}` : '/host/games/', { keepFocus: true, noScroll: true });
	}

	onMount(() => (gameId = page.url.searchParams.get('id') ?? ''));
</script>

<svelte:head>
	<title>Games | CloudJam</title>
	<meta property="og:title" content="Games | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

{#if gameId}
	{#key gameId}
		<GameWorkspace {gameId} close={() => open('')} />
	{/key}
{:else}
	<GameList open={(id) => open(id)} />
{/if}
