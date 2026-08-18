<script lang="ts">
	import { Glue, Submit } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import { GetRequestSchema } from '$lib/sdk/v1/play/game/game_pb';
	import type { Game } from '$lib/sdk/v1/play/game_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampDate } from '@bufbuild/protobuf/wkt';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import { onMount } from 'svelte';
	import ChallengeSection from './ChallengeSection.svelte';
	import GamePanel from './GamePanel.svelte';
	import TeamSection from './TeamSection.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';

	let gameId = $derived(page.params.id ?? '');

	const tabs = ['overview', 'teams', 'challenges'] as const;

	let error = $state('');
	let forbidden = $state(false);

	let game: Game | undefined = $state();
	let tab: (typeof tabs)[number] = $state('overview');

	function status(game: Game): string {
		const now = Date.now();
		if (game.from && timestampDate(game.from).getTime() > now) return 'upcoming';
		if (game.to && timestampDate(game.to).getTime() < now) return 'ended';
		return 'running';
	}

	function load() {
		Submit(
			async () => {
				game = (await Glue.game.get(create(GetRequestSchema, { id: gameId }))).game;
			},
			(e, _, f) => ((error = e), (forbidden = f))
		);
	}

	onMount(() => load());
</script>

<svelte:head>
	<title>{game?.name ?? 'Game'} | CloudJam</title>
	<meta property="og:title" content="Games | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<Button variant="ghost" size="icon" class="cursor-pointer" onclick={() => goto('/host/games')}>
			<ChevronLeftIcon />
		</Button>
		<h1 class="text-3xl opacity-80">{game?.name ?? 'Game'}</h1>
		{#if game}
			<Badge variant="secondary">{status(game)}</Badge>
			<Badge variant="outline">scope: {game.scope}</Badge>
		{/if}
		<Button
			variant="outline"
			class="ml-auto cursor-pointer font-mono text-xs"
			title="Copy game id"
			onclick={() => navigator.clipboard.writeText(gameId)}
		>
			<CopyIcon />
			{gameId}
		</Button>
	</div>

	<div class="flex flex-row gap-2 border-b">
		{#each tabs as name (name)}
			<button
				type="button"
				class="cursor-pointer border-b-2 px-4 py-2 capitalize transition-all duration-200 {tab === name
					? 'border-primary'
					: 'border-transparent opacity-60 hover:opacity-100'}"
				onclick={() => (tab = name)}
			>
				{name}
			</button>
		{/each}
	</div>

	{#if forbidden}
		<Alert.Root>
			<AlertCircleIcon />
			<Alert.Title>Permission Denied</Alert.Title>
			<Alert.Description>You are not allowed to open this game</Alert.Description>
		</Alert.Root>
	{:else if tab === 'overview'}
		{#if game}
			<GamePanel {game} refresh={() => load()} deleted={() => goto('/host/games')} />
		{/if}
	{:else if tab === 'teams'}
		<TeamSection {gameId} scope={game?.scope ?? ''} />
	{:else if tab === 'challenges'}
		<ChallengeSection {gameId} scope={game?.scope ?? ''} />
	{/if}

	{#if error}
		<Alert.Root variant="destructive">
			<AlertCircleIcon />
			<Alert.Title>Failed to load the game</Alert.Title>
			<Alert.Description>{error}</Alert.Description>
		</Alert.Root>
	{/if}
</div>
