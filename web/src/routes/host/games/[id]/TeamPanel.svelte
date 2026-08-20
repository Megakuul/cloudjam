<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import OptionalSelect from '$lib/components/custom/OptionalSelect.svelte';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import * as Select from '$lib/components/shad/select';
	import { Separator } from '$lib/components/shad/separator';
	import { ListRequestSchema as ListUsersRequestSchema } from '$lib/sdk/v1/admin/user/user_pb';
	import type { User } from '$lib/sdk/v1/admin/user_pb';
	import { DeleteRequestSchema, UpdateRequestSchema } from '$lib/sdk/v1/play/team/team_pb';
	import { PlayerSchema, type Player, type Team } from '$lib/sdk/v1/play/team_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import XIcon from '@lucide/svelte/icons/x';
	import { onMount } from 'svelte';

	let { team, refresh }: { team: Team; refresh: () => void } = $props();

	let name = $derived(team.name);
	let players: { [key: string]: Player } = $derived({ ...team.players });
	let confirmDelete = $state(false);

	let users: User[] = $state([]);
	let player = $state('');

	let updateState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let removeState: SubmitState = $state({ error: '', loading: false, forbidden: false });

	function save() {
		Submit(async () => {
			await Glue.team.update(create(UpdateRequestSchema, { mod: { ...team, name: name, players: players } }));
			refresh();
		}, updateState);
	}

	let usersState: SubmitState = $state({ error: '', loading: false, forbidden: false });

	onMount(() =>
		Submit(async () => {
			users = (await Glue.user.list(create(ListUsersRequestSchema, { limit: 100 }))).users;
		}, usersState)
	);
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title class="text-2xl">{team.name}</Card.Title>
		<Card.Description>{Object.keys(team.players).length} players</Card.Description>
		<div class="flex flex-row flex-wrap gap-1">
			<Badge variant="secondary">score: {team.score}</Badge>
			<Badge variant="outline">scope: {team.scope}</Badge>
			<Badge variant="outline" class="font-mono">{team.id}</Badge>
		</div>
	</Card.Header>
	<Card.Content class="flex flex-col gap-6">
		<div class="flex flex-col gap-2">
			<Card.Title>Name</Card.Title>
			{#if updateState.forbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to update this team.</p>
			{:else}
				<form class="flex flex-row items-center gap-2" onsubmit={() => save()}>
					<Input class="max-w-96" bind:value={name} placeholder="Name of the team" />
					<Button type="submit" variant="outline" class="cursor-pointer" disabled={updateState.loading}>Save</Button>
				</form>
				{#if updateState.error}
					<p class="text-destructive text-xs">{updateState.error}</p>
				{/if}
			{/if}
		</div>

		<Separator />

		<div class="flex flex-col gap-2">
			<Card.Title>Players</Card.Title>
			<div class="flex flex-row flex-wrap gap-1">
				{#each Object.values(players) as member (member.id)}
					<Badge variant="secondary" class="gap-1">
						{member.username}
						<button
							type="button"
							class="cursor-pointer"
							aria-label="Remove {member.username}"
							onclick={() => {
								delete players[member.id];
								save();
							}}
						>
							<XIcon class="size-3" />
						</button>
					</Badge>
				{:else}
					<p class="text-muted-foreground text-sm italic">No players attached yet.</p>
				{/each}
			</div>
			<div class="flex flex-row items-center gap-2">
				<OptionalSelect
					bind:value={player}
					placeholder="Select a player"
					suggestions={users.map((user) => ({ id: user.id, title: user.username }))}
				/>

				<Button
					variant="outline"
					class="cursor-pointer"
					disabled={updateState.loading || !player || Boolean(players[player])}
					onclick={() => {
						const user = users.find((user) => user.id === player);
						if (!user) return;
						players[user.id] = create(PlayerSchema, { id: user.id, pubId: user.pubId, username: user.username });
						save();
					}}
				>
					Attach
				</Button>
			</div>
		</div>

		<Separator />

		<div class="flex flex-col gap-2">
			<Card.Title>Danger Zone</Card.Title>
			{#if removeState.forbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to delete this team.</p>
			{:else}
				<div class="flex flex-row items-center gap-2">
					{#if confirmDelete}
						<Button
							variant="destructive"
							class="cursor-pointer"
							disabled={removeState.loading}
							onclick={() =>
								Submit(async () => {
									await Glue.team.delete(create(DeleteRequestSchema, { gameId: team.gameId, id: team.id }));
									refresh();
								}, removeState)}
						>
							Yes, delete {team.name}
						</Button>
						<Button variant="outline" class="cursor-pointer" onclick={() => (confirmDelete = false)}>Cancel</Button>
					{:else}
						<Button variant="destructive" class="cursor-pointer" onclick={() => (confirmDelete = true)}>
							Delete Team
						</Button>
					{/if}
				</div>
				{#if removeState.error}
					<Alert.Root variant="destructive">
						<AlertCircleIcon />
						<Alert.Title>Failed to delete team</Alert.Title>
						<Alert.Description>{removeState.error}</Alert.Description>
					</Alert.Root>
				{/if}
			{/if}
		</div>
	</Card.Content>
</Card.Root>
