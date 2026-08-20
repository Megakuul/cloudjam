<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Button } from '$lib/components/shad/button';
	import * as Table from '$lib/components/shad/table';
	import { ListRequestSchema } from '$lib/sdk/v1/play/team/team_pb';
	import type { Team } from '$lib/sdk/v1/play/team_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import { onMount } from 'svelte';
	import CreateTeam from './CreateTeam.svelte';
	import TeamPanel from './TeamPanel.svelte';

	let { gameId }: { gameId: string } = $props();

	const limit = 100;

	let teams: Team[] = $state([]);
	let exhausted = $state(true);

	let selected: Team | undefined = $state();
	let creating = $state(false);

	let teamsState: SubmitState = $state({ error: '', forbidden: false, loading: false });

	function load(startAfter?: string) {
		Submit(async () => {
			const resp = await Glue.team.list(
				create(ListRequestSchema, { gameId: gameId, limit: limit, startAfter: startAfter })
			);
			teams = startAfter ? [...teams, ...resp.teams] : resp.teams;
			exhausted = resp.teams.length < limit;
		}, teamsState);
	}

	onMount(() => load());
</script>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<h2 class="text-xl opacity-80">Teams</h2>
		<Button variant="outline" class="ml-auto cursor-pointer" onclick={() => (creating = !creating)}>
			<PlusIcon /> Create Team
		</Button>
	</div>

	{#if creating}
		<CreateTeam {gameId} oncreated={() => load()} />
	{/if}

	{#if teamsState.forbidden}
		<Alert.Root>
			<AlertCircleIcon />
			<Alert.Title>Permission Denied</Alert.Title>
			<Alert.Description>You are not allowed to list the teams of this game</Alert.Description>
		</Alert.Root>
	{:else}
		<Table.Root>
			<Table.Header>
				<Table.Row>
					<Table.Head>Name</Table.Head>
					<Table.Head>Players</Table.Head>
					<Table.Head>Score</Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each teams as team (team.id)}
					<Table.Row class="cursor-pointer" onclick={() => (selected = selected?.id === team.id ? undefined : team)}>
						<Table.Cell class="font-medium">{team.name}</Table.Cell>
						<Table.Cell>
							{Object.values(team.players)
								.map((player) => player.username)
								.join(', ')}
						</Table.Cell>
						<Table.Cell>{team.score}</Table.Cell>
					</Table.Row>
				{:else}
					<Table.Row>
						<Table.Cell colspan={3}>
							<p class="text-muted-foreground p-4 text-sm italic">
								{teamsState.loading ? 'Loading teams…' : 'No teams in this game yet.'}
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
				disabled={teamsState.loading}
				onclick={() => load(teams.at(-1)?.id)}
			>
				Load More
			</Button>
		{/if}
	{/if}

	{#if selected}
		{#key selected.id}
			<TeamPanel team={selected} refresh={() => load()} />
		{/key}
	{/if}

	{#if teamsState.error}
		<Alert.Root variant="destructive">
			<AlertCircleIcon />
			<Alert.Title>Failed to load teams</Alert.Title>
			<Alert.Description>{teamsState.error}</Alert.Description>
		</Alert.Root>
	{/if}
</div>
