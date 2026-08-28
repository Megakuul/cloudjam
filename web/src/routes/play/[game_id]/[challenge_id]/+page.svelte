<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Glue, Submit, type SubmitState } from '$lib';
	import * as Card from '$lib/components/shad/card';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import { GetRequestSchema as GetChallengeRequestSchema } from '$lib/sdk/v1/play/challenge/challenge_pb';
	import type { Challenge } from '$lib/sdk/v1/play/challenge_pb';
	import { GetRequestSchema as GetGameRequestSchema } from '$lib/sdk/v1/play/game/game_pb';
	import type { Game } from '$lib/sdk/v1/play/game_pb';
	import { GetRequestSchema as GetTeamRequestSchema } from '$lib/sdk/v1/play/team/team_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampDate } from '@bufbuild/protobuf/wkt';
	import { ChevronLeftIcon, LockIcon } from '@lucide/svelte';
	import type { Team } from '$lib/sdk/v1/play/team_pb';
	import { GetRequestSchema as GetDefinitionRequestSchema } from '$lib/sdk/v1/cloud/definition/definition_pb';
	import type { Definition } from '$lib/sdk/v1/cloud/definition_pb';
	import PlayChallenge from './PlayChallenge.svelte';
	import { toSvg } from 'jdenticon';

	const reloadInterval = 5000;

	let gameId: string | undefined = $derived(page.params.game_id);
	let gameState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let game: Game | undefined = $state();
	$effect(() => {
		if (!gameId) return;
		Submit(async () => {
			game = (
				await Glue.game.get(
					create(GetGameRequestSchema, {
						id: gameId
					})
				)
			).game;
		}, gameState);
	});

	let teamId: string | undefined = $state('');
	let teamState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let team: Team | undefined = $state();
	$effect(() => {
		if (!teamId) return undefined;
		if (!gameId) return undefined;
		Submit(async () => {
			team = (
				await Glue.team.get(
					create(GetTeamRequestSchema, {
						gameId: gameId,
						id: teamId
					})
				)
			).team;
		}, teamState);
	});

	let definitionId: string | undefined = $state();
	let definitionProviderId: string | undefined = $state();
	let definitionState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let definition: Definition | undefined = $state();
	$effect(() => {
		if (!definitionId) return undefined;
		if (!definitionProviderId) return undefined;
		Submit(async () => {
			definition = (
				await Glue.definition.get(
					create(GetDefinitionRequestSchema, {
						providerId: definitionProviderId,
						id: definitionId
					})
				)
			).definition;
		}, definitionState);
	});

	let challengeTimeout: ReturnType<typeof setTimeout>;
	let nextInterval: Date = $state(new Date());

	async function reloadChallenge() {
		clearTimeout(challengeTimeout);
		async function poll() {
			await Submit(async () => {
				challenge = (
					await Glue.challenge.get(
						create(GetChallengeRequestSchema, {
							gameId: gameId,
							id: challengeId
						})
					)
				).challenge;
			}, challengeState);
			teamId = challenge?.teamId;
			definitionId = challenge?.definitionId;
			definitionProviderId = challenge?.definitionProviderId;
			nextInterval = new Date(Date.now() + reloadInterval);
			challengeTimeout = setTimeout(poll, reloadInterval);
		}
		poll();
	}

	let challengeId: string | undefined = $derived(page.params.challenge_id);
	let challengeState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let challenge: Challenge | undefined = $state();
	$effect(() => {
		if (!gameId) return undefined;
		if (!challengeId) return undefined;
		reloadChallenge();
	});

	let score = $derived(challenge?.scoreEvents.reduce((sum, event) => sum + event.change, 0) ?? 0);

	let active = $state(false);
	$effect(() => {
		if (!game) {
			active = false;
			return;
		}
		const updateActive = () => {
			active = timestampDate(game!.from!).getTime() < Date.now() && timestampDate(game!.to!).getTime() > Date.now();
		};
		updateActive();
		const interval = setInterval(updateActive, 1000);
		return () => clearInterval(interval);
	});
</script>

<svelte:head>
	<title>{challenge?.title || definition?.name || 'Challenge'} | CloudJam</title>
	<meta property="og:title" content="Challenge | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<Button variant="ghost" size="icon" class="cursor-pointer" onclick={() => goto(`/play/${page.params.game_id}`)}>
			<ChevronLeftIcon />
		</Button>
		<h1 class="text-3xl opacity-80">{challenge?.title || definition?.name || 'Challenge'}</h1>
		{#if definition}
			<Badge variant="secondary">{definition?.name}</Badge>
		{/if}
		{#if !active}
			<Badge variant="secondary">
				<LockIcon />
				Locked
			</Badge>
		{/if}
		<Badge variant="outline">score: {score}</Badge>
	</div>

	<Card.Root class="bg-accent-600/20 gap-2 p-4">
		<Card.Title class="flex flex-row items-center gap-2 text-2xl">
			{team?.name}
			<Badge variant="default">
				score: {team?.score}
			</Badge>
		</Card.Title>
		<Card.Description>
			{#each Object.values(team?.players ?? {}) as player (player.id)}
				<Badge variant="outline">
					<img
						alt="user profile"
						src={`data:image/svg+xml;base64,${btoa(toSvg(player.pubId, 16))}`}
						height="3rem"
						class="bg-primary/5 rounded-lg"
					/>
					{player.username}
				</Badge>
			{/each}
		</Card.Description>
	</Card.Root>
	{#if challenge}
		<PlayChallenge {challenge} {active} {nextInterval} refresh={() => reloadChallenge()} />
	{/if}
</div>
