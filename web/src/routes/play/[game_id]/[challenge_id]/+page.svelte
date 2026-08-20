<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Glue, Submit, type SubmitState } from '$lib';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import { GetRequestSchema as GetChallengeRequestSchema } from '$lib/sdk/v1/play/challenge/challenge_pb';
	import type { Challenge } from '$lib/sdk/v1/play/challenge_pb';
	import { GetRequestSchema as GetGameRequestSchema } from '$lib/sdk/v1/play/game/game_pb';
	import type { Game } from '$lib/sdk/v1/play/game_pb';
	import { GetRequestSchema as GetTeamRequestSchema } from '$lib/sdk/v1/play/team/team_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampDate } from '@bufbuild/protobuf/wkt';
	import { ChevronLeftIcon } from '@lucide/svelte';
	import type { Team } from '$lib/sdk/v1/play/team_pb';
	import { GetRequestSchema as GetDefinitionRequestSchema } from '$lib/sdk/v1/cloud/definition/definition_pb';
	import type { Definition } from '$lib/sdk/v1/cloud/definition_pb';
	import PlayChallenge from './PlayChallenge.svelte';

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

	let challengeState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let challenge: Challenge | undefined = $state();
	$effect(() => {
		if (!game) return undefined;

		let timeoutId: ReturnType<typeof setTimeout>;

		async function poll() {
			await Submit(async () => {
				challenge = (
					await Glue.challenge.get(
						create(GetChallengeRequestSchema, {
							gameId: game?.id,
							id: page.params.challenge_id
						})
					)
				).challenge;
			}, challengeState);
			timeoutId = setTimeout(poll, 5000);
		}
		poll();

		return () => clearTimeout(timeoutId);
	});

	let teamState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let team: Team | undefined = $state();
	$effect(() => {
		if (!challenge) return undefined;
		Submit(async () => {
			team = (
				await Glue.team.get(
					create(GetTeamRequestSchema, {
						gameId: challenge?.gameId,
						id: challenge?.teamId
					})
				)
			).team;
		}, teamState);
	});

	let definitionState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let definition: Definition | undefined = $state();
	$effect(() => {
		if (!challenge) return undefined;
		Submit(async () => {
			definition = (
				await Glue.definition.get(
					create(GetDefinitionRequestSchema, {
						providerId: challenge?.definitionProviderId,
						id: challenge?.definitionId
					})
				)
			).definition;
		}, definitionState);
	});

	let score = $derived(challenge?.scoreEvents.reduce((sum, event) => sum + event.change, 0) ?? 0);

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
		<Badge variant="outline">score: {score}</Badge>
	</div>

	{#if challenge}
		<PlayChallenge {challenge} refresh={() => {}} />
	{/if}
</div>
