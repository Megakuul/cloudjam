<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Table from '$lib/components/shad/table';
	import { ListRequestSchema } from '$lib/sdk/v1/play/challenge/challenge_pb';
	import type { Challenge } from '$lib/sdk/v1/play/challenge_pb';
	import { ListRequestSchema as ListTeamRequestSchema } from '$lib/sdk/v1/play/team/team_pb';
	import type { Team } from '$lib/sdk/v1/play/team_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import { onMount } from 'svelte';
	import ChallengePanel from './ChallengePanel.svelte';
	import CreateChallenge from './CreateChallenge.svelte';

	let { gameId }: { gameId: string } = $props();

	const limit = 100;

	let challenges: Challenge[] = $state([]);
	let exhausted = $state(true);

	let teams: Team[] = $state([]);

	let selected: Challenge | undefined = $state();
	let creating = $state(false);

	const score = (challenge: Challenge) => challenge.scoreEvents.reduce((sum, event) => sum + event.change, 0);

	function teamName(id: string): string {
		return teams.find((team) => team.id === id)?.name ?? id;
	}

	let challengesState: SubmitState = $state({ error: '', loading: false, forbidden: false });

	function load(startAfter?: string) {
		selected = undefined;
		Submit(async () => {
			const resp = await Glue.challenge.list(
				create(ListRequestSchema, { gameId: gameId, limit: limit, startAfter: startAfter })
			);
			challenges = startAfter ? [...challenges, ...resp.challenges] : resp.challenges;
			exhausted = resp.challenges.length < limit;
		}, challengesState);
	}

	let teamsState: SubmitState = $state({ error: '', loading: false, forbidden: false });

	onMount(() => {
		load();
		Submit(async () => {
			teams = (await Glue.team.list(create(ListTeamRequestSchema, { gameId: gameId, limit: limit }))).teams;
		}, teamsState);
	});
</script>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<h2 class="text-xl opacity-80">Challenges</h2>
		<Button variant="outline" class="ml-auto cursor-pointer" onclick={() => (creating = !creating)}>
			<PlusIcon /> Provision Challenges
		</Button>
	</div>

	{#if creating}
		<CreateChallenge {gameId} {teams} oncreated={() => load()} />
	{/if}

	{#if challengesState.forbidden}
		<Alert.Root>
			<AlertCircleIcon />
			<Alert.Title>Permission Denied</Alert.Title>
			<Alert.Description>You are not allowed to list the challenges of this game</Alert.Description>
		</Alert.Root>
	{:else}
		<Table.Root>
			<Table.Header>
				<Table.Row>
					<Table.Head>Title</Table.Head>
					<Table.Head>Team</Table.Head>
					<Table.Head>Score</Table.Head>
					<Table.Head>Errors</Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each challenges as challenge (challenge.id)}
					<Table.Row
						class="cursor-pointer"
						onclick={() => (selected = selected?.id === challenge.id ? undefined : challenge)}
					>
						<Table.Cell class="font-medium">{challenge.title || 'not started'}</Table.Cell>
						<Table.Cell>{teamName(challenge.teamId)}</Table.Cell>
						<Table.Cell>{score(challenge)}</Table.Cell>
						<Table.Cell>
							{#if challenge.errors.length}
								<Badge variant="destructive">{challenge.errors.length}</Badge>
							{/if}
						</Table.Cell>
					</Table.Row>
				{:else}
					<Table.Row>
						<Table.Cell colspan={4}>
							<p class="text-muted-foreground p-4 text-sm italic">
								{challengesState.loading ? 'Loading challenges…' : 'No challenges handed out in this game yet.'}
							</p>
						</Table.Cell>
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
		{#if !exhausted}
			<Button
				variant="outline"
				class="cursor-pointer self-center"
				disabled={challengesState.loading}
				onclick={() => load(challenges.at(-1)?.id)}
			>
				Load More
			</Button>
		{/if}
	{/if}

	{#if selected}
		{#key selected.id}
			<ChallengePanel challenge={selected} {teams} refresh={() => load()} />
		{/key}
	{/if}

	{#if challengesState.error}
		<Alert.Root variant="destructive">
			<AlertCircleIcon />
			<Alert.Title>Failed to load challenges</Alert.Title>
			<Alert.Description>{challengesState.error}</Alert.Description>
		</Alert.Root>
	{/if}
</div>
