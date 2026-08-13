<script lang="ts">
	import { Glue, Submit } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import * as Select from '$lib/components/shad/select';
	import { Separator } from '$lib/components/shad/separator';
	import * as Table from '$lib/components/shad/table';
	import { DeleteRequestSchema, UpdateRequestSchema } from '$lib/sdk/v1/play/challenge/challenge_pb';
	import type { Challenge } from '$lib/sdk/v1/play/challenge_pb';
	import type { Team } from '$lib/sdk/v1/play/team_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampDate } from '@bufbuild/protobuf/wkt';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';

	let { challenge, teams, refresh }: { challenge: Challenge; teams: Team[]; refresh: () => void } = $props();

	type section = { error: string; loading: boolean; forbidden: boolean };
	const setter = (s: section) => (e: string, l: boolean, f: boolean) => (
		(s.error = e),
		(s.loading = l),
		(s.forbidden = f)
	);

	// the panel is remounted (keyed) per challenge, capturing the initial value is intended.
	// svelte-ignore state_referenced_locally
	let teamId = $state(challenge.teamId);
	let confirmDelete = $state(false);

	let update: section = $state({ error: '', loading: false, forbidden: false });
	let remove: section = $state({ error: '', loading: false, forbidden: false });
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title class="text-2xl">{challenge.title || 'Not started yet'}</Card.Title>
		<Card.Description>{challenge.description.join(' ')}</Card.Description>
		<div class="flex flex-row flex-wrap gap-1">
			<Badge variant="secondary">
				score: {challenge.scoreEvents.reduce((sum, event) => sum + event.change, 0)}
			</Badge>
			<Badge variant="outline">scope: {challenge.scope}</Badge>
			<Badge variant="outline" class="font-mono">{challenge.id}</Badge>
		</div>
	</Card.Header>
	<Card.Content class="flex flex-col gap-6">
		{#each challenge.errors as message, index (index)}
			<Alert.Root variant="destructive">
				<AlertCircleIcon />
				<Alert.Title>Challenge reported an error</Alert.Title>
				<Alert.Description class="font-mono text-xs break-all">{message}</Alert.Description>
			</Alert.Root>
		{/each}

		<div class="flex flex-col gap-2">
			<Card.Title>Team</Card.Title>
			{#if update.forbidden}
				<p class="text-sm text-muted-foreground italic">You are not allowed to update this challenge.</p>
			{:else}
				<div class="flex flex-row items-center gap-2">
					<Select.Root type="single" bind:value={teamId}>
						<Select.Trigger class="w-72 cursor-pointer">
							{teams.find((team) => team.id === teamId)?.name ?? 'Select a team'}
						</Select.Trigger>
						<Select.Content>
							{#each teams as team (team.id)}
								<Select.Item value={team.id} label={team.name}>{team.name}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
					<Button
						variant="outline"
						class="cursor-pointer"
						disabled={update.loading || !teamId || teamId === challenge.teamId}
						onclick={() =>
							Submit(async () => {
								await Glue.challenge.update(create(UpdateRequestSchema, { mod: { ...challenge, teamId: teamId } }));
								refresh();
							}, setter(update))}
					>
						Reassign
					</Button>
				</div>
				{#if update.error}
					<p class="text-xs text-destructive">{update.error}</p>
				{/if}
			{/if}
		</div>

		{#if challenge.scoreEvents.length}
			<Separator />

			<div class="flex flex-col gap-2">
				<Card.Title>Score Events</Card.Title>
				<Table.Root>
					<Table.Header>
						<Table.Row>
							<Table.Head>Time</Table.Head>
							<Table.Head>Event</Table.Head>
							<Table.Head>Change</Table.Head>
						</Table.Row>
					</Table.Header>
					<Table.Body>
						{#each challenge.scoreEvents as event, index (index)}
							<Table.Row>
								<Table.Cell>{event.timestamp ? timestampDate(event.timestamp).toLocaleString() : ''}</Table.Cell>
								<Table.Cell>{event.text}</Table.Cell>
								<Table.Cell>{event.change}</Table.Cell>
							</Table.Row>
						{/each}
					</Table.Body>
				</Table.Root>
			</div>
		{/if}

		<Separator />

		<div class="flex flex-col gap-2">
			<Card.Title>Danger Zone</Card.Title>
			{#if remove.forbidden}
				<p class="text-sm text-muted-foreground italic">You are not allowed to delete this challenge.</p>
			{:else}
				<div class="flex flex-row items-center gap-2">
					{#if confirmDelete}
						<Button
							variant="destructive"
							class="cursor-pointer"
							disabled={remove.loading}
							onclick={() =>
								Submit(async () => {
									await Glue.challenge.delete(
										create(DeleteRequestSchema, { gameId: challenge.gameId, id: challenge.id })
									);
									refresh();
								}, setter(remove))}
						>
							Yes, delete this challenge
						</Button>
						<Button variant="outline" class="cursor-pointer" onclick={() => (confirmDelete = false)}>Cancel</Button>
					{:else}
						<Button variant="destructive" class="cursor-pointer" onclick={() => (confirmDelete = true)}>
							Delete Challenge
						</Button>
					{/if}
				</div>
				{#if remove.error}
					<Alert.Root variant="destructive">
						<AlertCircleIcon />
						<Alert.Title>Failed to delete challenge</Alert.Title>
						<Alert.Description>{remove.error}</Alert.Description>
					</Alert.Root>
				{/if}
			{/if}
		</div>
	</Card.Content>
</Card.Root>
