<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Glue, Submit } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import * as Table from '$lib/components/shad/table';
	import { ListRequestSchema } from '$lib/sdk/v1/play/challenge/challenge_pb';
	import type { Challenge } from '$lib/sdk/v1/play/challenge_pb';
	import { GetRequestSchema } from '$lib/sdk/v1/play/game/game_pb';
	import type { Game } from '$lib/sdk/v1/play/game_pb';
	import { ListRequestSchema as ListTeamsRequestSchema } from '$lib/sdk/v1/play/team/team_pb';
	import type { Team } from '$lib/sdk/v1/play/team_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampDate } from '@bufbuild/protobuf/wkt';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import { onMount } from 'svelte';
	import PlayChallenge from './PlayChallenge.svelte';

	const limit = 100;

	let error = $state('');
	let loading = $state(false);
	let forbidden = $state(false);

	let gameId = $derived(page.params.id ?? '');

	let game: Game | undefined = $state();
	let teams: Team[] = $state([]);
	let challenges: Challenge[] = $state([]);
	let selected: Challenge | undefined = $state();

	const score = (challenge: Challenge) => challenge.scoreEvents.reduce((sum, event) => sum + event.change, 0);

	function status(game: Game): string {
		const now = Date.now();
		if (game.from && timestampDate(game.from).getTime() > now) return 'upcoming';
		if (game.to && timestampDate(game.to).getTime() < now) return 'ended';
		return 'running';
	}

	function load() {
		selected = undefined;
		Submit(
			async () => {
				game = (await Glue.game.get(create(GetRequestSchema, { id: gameId }))).game;
				challenges = (await Glue.challenge.list(create(ListRequestSchema, { gameId: gameId, limit: limit })))
					.challenges;
			},
			(e, l, f) => ((error = e), (loading = l), (forbidden = f))
		);
		// teams are shown as scoreboard; without team access the section stays empty.
		Submit(
			async () => {
				teams = (await Glue.team.list(create(ListTeamsRequestSchema, { gameId: gameId, limit: limit }))).teams;
			},
			() => {}
		);
	}

	onMount(() => load());
</script>

<svelte:head>
	<title>Play | CloudJam</title>
	<meta property="og:title" content="Play | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<h1 class="text-3xl opacity-80">{game?.name ?? 'Game'}</h1>
		{#if game}
			<Badge variant="secondary">{status(game)}</Badge>
		{/if}
		<Button variant="outline" class="ml-auto cursor-pointer" onclick={() => goto('/play')}>Leave</Button>
	</div>

	{#if game}
		<p class="text-muted-foreground text-sm">
			{game.description}
			{#if game.from && game.to}
				· {timestampDate(game.from).toLocaleString()} – {timestampDate(game.to).toLocaleString()}
			{/if}
		</p>
	{/if}

	{#if forbidden}
		<Alert.Root>
			<AlertCircleIcon />
			<Alert.Title>Permission Denied</Alert.Title>
			<Alert.Description>You are not part of this game</Alert.Description>
		</Alert.Root>
	{:else}
		<div class="grid gap-4 md:grid-cols-3">
			{#each challenges as challenge (challenge.id)}
				<button
					type="button"
					class="cursor-pointer text-left"
					onclick={() => (selected = selected?.id === challenge.id ? undefined : challenge)}
				>
					<Card.Root class="h-full transition-all duration-200 hover:bg-slate-50/5">
						<Card.Header>
							<Card.Title class="text-xl">{challenge.title || 'Not started yet'}</Card.Title>
							<Card.Description>{challenge.description.at(0) ?? ''}</Card.Description>
							<div class="flex flex-row flex-wrap gap-1">
								<Badge variant="secondary">score: {score(challenge)}</Badge>
								{#if challenge.errors.length}
									<Badge variant="destructive">{challenge.errors.length} errors</Badge>
								{/if}
							</div>
						</Card.Header>
					</Card.Root>
				</button>
			{:else}
				{#if !loading}
					<p class="text-muted-foreground text-sm italic">No challenges handed out to you in this game.</p>
				{/if}
			{/each}
		</div>

		{#if selected}
			{#key selected.id}
				<PlayChallenge challenge={selected} refresh={() => load()} />
			{/key}
		{/if}

		{#if teams.length}
			<Card.Root class="w-full">
				<Card.Header>
					<Card.Title>Scoreboard</Card.Title>
					<Card.Description>Teams competing in this game.</Card.Description>
				</Card.Header>
				<Card.Content>
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head>Team</Table.Head>
								<Table.Head>Players</Table.Head>
								<Table.Head>Score</Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#each [...teams].sort((a, b) => b.score - a.score) as team (team.id)}
								<Table.Row>
									<Table.Cell class="font-medium">{team.name}</Table.Cell>
									<Table.Cell>
										{Object.values(team.players)
											.map((player) => player.username)
											.join(', ')}
									</Table.Cell>
									<Table.Cell>{team.score}</Table.Cell>
								</Table.Row>
							{/each}
						</Table.Body>
					</Table.Root>
				</Card.Content>
			</Card.Root>
		{/if}
	{/if}

	{#if error}
		<Alert.Root variant="destructive">
			<AlertCircleIcon />
			<Alert.Title>Failed to load the game</Alert.Title>
			<Alert.Description>{error}</Alert.Description>
		</Alert.Root>
	{/if}
</div>
