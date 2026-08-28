<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getSubject, Glue, Submit, type SubmitState } from '$lib';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { ListRequestSchema as ListChallengeRequestSchema } from '$lib/sdk/v1/play/challenge/challenge_pb';
	import type { Challenge } from '$lib/sdk/v1/play/challenge_pb';
	import { GetRequestSchema as GetGameRequestSchema } from '$lib/sdk/v1/play/game/game_pb';
	import type { Game } from '$lib/sdk/v1/play/game_pb';
	import { ListRequestSchema as ListTeamsRequestSchema } from '$lib/sdk/v1/play/team/team_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampDate } from '@bufbuild/protobuf/wkt';
	import { ChevronLeftIcon, LayersIcon } from '@lucide/svelte';
	import type { Team } from '$lib/sdk/v1/play/team_pb';

	let gameState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let game: Game | undefined = $state();
	$effect(() => {
		Submit(async () => {
			game = (
				await Glue.game.get(
					create(GetGameRequestSchema, {
						id: page.params.game_id
					})
				)
			).game;
		}, gameState);
	});

	let challengesState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let challenges: Challenge[] = $state([]);
	$effect(() => {
		if (!game) return undefined;
		Submit(async () => {
			challenges = (
				await Glue.challenge.list(
					create(ListChallengeRequestSchema, {
						gameId: game?.id,
						limit: 100
					})
				)
			).challenges;
		}, challengesState);
	});

	let teamsState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let teams: Team[] = $state([]);
	$effect(() => {
		if (!game) return undefined;
		Submit(async () => {
			teams = (
				await Glue.team.list(
					create(ListTeamsRequestSchema, {
						gameId: game?.id,
						limit: 100
					})
				)
			).teams.sort((a, b) => a.score - b.score);
		}, teamsState);
	});

	let userTeam: Team | undefined = $state();
	$effect(() => {
		const id = getSubject();
		for (const team of teams) {
			console.log(team.players);
			if (id in team.players) {
				userTeam = team;
				return;
			}
		}
		userTeam = undefined;
	});

	let active = $state(false);
	$effect(() => {
		if (!game) {
			active = false;
			return;
		}
		const updateActive = () =>
			(active = timestampDate(game!.from!).getTime() < Date.now() && timestampDate(game!.to!).getTime() > Date.now());
		const interval = setInterval(updateActive, 1000);
		updateActive();
		return () => clearInterval(interval);
	});
</script>

<svelte:head>
	<title>{game?.name ?? 'Game'} | CloudJam</title>
	<meta property="og:title" content="Game | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<Button variant="ghost" size="icon" class="cursor-pointer" onclick={() => goto(`/play`)}>
			<ChevronLeftIcon />
		</Button>
		<h1 class="text-3xl opacity-80">{game?.name ?? 'Game'}</h1>
		{#if active}
			<Badge variant="secondary">running</Badge>
		{:else}
			<Badge variant="outline">ended</Badge>
		{/if}
	</div>

	<p class="text-muted-foreground text-sm">
		{game?.description}
	</p>

	{#if userTeam}
		<div class="flex flex-row flex-wrap gap-4">
			{#each challenges.filter((challenge) => challenge.teamId === userTeam!.id) as challenge (challenge.id)}
				<button
					type="button"
					class="w-full cursor-pointer text-left sm:w-80"
					onclick={() => goto(`/play/${game?.id}/${challenge.id}`)}
				>
					<Card.Root class="h-full hover:bg-slate-50/5">
						<Card.Header>
							<LayersIcon class="size-8" />
							<Card.Title class="text-xl">{challenge.title}</Card.Title>
							<Card.Description>
								{challenge.description.join('\n').slice(0, 20)}
								{challenge.description.length < 1 ? '' : '...'}
							</Card.Description>
							<div class="flex flex-row flex-wrap gap-1">
								{#if !challenge.title}
									<Badge variant="secondary">not discovered</Badge>
								{/if}
								{#if challenge.error}
									<Badge variant="destructive">challenge defect</Badge>
								{/if}
							</div>
						</Card.Header>
					</Card.Root>
				</button>
			{/each}
		</div>
	{/if}
</div>

<!-- <h1>TODO: here will be an epic leaderboard</h1> -->
<!-- <div class="flex flex-col"> -->
<!-- 	{#each teams as team (team.id)} -->
<!-- 		<div>{team.name}: {team.scope}</div> -->
<!-- 	{/each} -->
<!-- </div> -->
