<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getPubId, getSubject, Glue, Submit, type SubmitState } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import * as Table from '$lib/components/shad/table';
	import {
		GetRequestSchema as GetChallengeRequestSchema,
		ListRequestSchema as ListChallengeRequestSchema
	} from '$lib/sdk/v1/play/challenge/challenge_pb';
	import type { Challenge } from '$lib/sdk/v1/play/challenge_pb';
	import { GetRequestSchema as GetGameRequestSchema } from '$lib/sdk/v1/play/game/game_pb';
	import type { Game } from '$lib/sdk/v1/play/game_pb';
	import {
		GetRequestSchema as GetTeamRequestSchema,
		ListRequestSchema as ListTeamsRequestSchema
	} from '$lib/sdk/v1/play/team/team_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampDate } from '@bufbuild/protobuf/wkt';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import { onMount } from 'svelte';
	import { ChevronLeftIcon, LayersIcon } from '@lucide/svelte';
	import type { Team } from '$lib/sdk/v1/play/team_pb';
	import { GetRequestSchema as GetDefinitionRequestSchema } from '$lib/sdk/v1/cloud/definition/definition_pb';

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
		const interval = setInterval(() => {
			active = timestampDate(game!.from!).getTime() > Date.now() && timestampDate(game!.to!).getTime() < Date.now();
		}, 1000);
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
</div>

{#if userTeam}
	<div class="flex flex-row flex-wrap gap-4">
		{#each challenges.filter((challenge) => challenge.teamId === userTeam!.id) as challenge (challenge.id)}
			<button
				type="button"
				class="w-full cursor-pointer text-left sm:w-80"
				onclick={() => goto(`/play/${game?.id}/${challenge.id}`)}
			>
				<Card.Root class="h-full transition-all duration-200 hover:bg-slate-50/5">
					<Card.Header>
						<LayersIcon class="size-8" />
						<Card.Title class="text-xl">{challenge.title}</Card.Title>
						<Card.Description>{challenge.description}</Card.Description>
						<div class="flex flex-row flex-wrap gap-1">
							<Badge variant="secondary">Ananas</Badge>
						</div>
					</Card.Header>
				</Card.Root>
			</button>
		{/each}
	</div>
{/if}

<h1>TODO: here would be an epic leaderboard</h1>
<div class="flex flex-col">
	{#each teams as team (team.id)}
		<div>{team.name}: {team.scope}</div>
	{/each}
</div>
