<script lang="ts">
	import { Glue, Submit } from '$lib';
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

	type section = { error: string; loading: boolean; forbidden: boolean };
	const setter = (s: section) => (e: string, l: boolean, f: boolean) => (
		(s.error = e),
		(s.loading = l),
		(s.forbidden = f)
	);

	// the panel is remounted (keyed) per team, capturing the initial values is intended.
	// svelte-ignore state_referenced_locally
	let name = $state(team.name);
	// svelte-ignore state_referenced_locally
	let players: { [key: string]: Player } = $state({ ...team.players });
	let confirmDelete = $state(false);

	// users are only used to attach players; without user access the section is hidden.
	let users: User[] = $state([]);
	let usersForbidden = $state(false);
	let player = $state('');

	let update: section = $state({ error: '', loading: false, forbidden: false });
	let remove: section = $state({ error: '', loading: false, forbidden: false });

	function save() {
		Submit(async () => {
			await Glue.team.update(create(UpdateRequestSchema, { mod: { ...team, name: name, players: players } }));
			refresh();
		}, setter(update));
	}

	onMount(() =>
		Submit(
			async () => {
				users = (await Glue.user.list(create(ListUsersRequestSchema, { limit: 100 }))).users;
			},
			(_, __, f) => (usersForbidden = f)
		)
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
			{#if update.forbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to update this team.</p>
			{:else}
				<form class="flex flex-row items-center gap-2" onsubmit={() => save()}>
					<Input class="max-w-96" bind:value={name} placeholder="Name of the team" />
					<Button type="submit" variant="outline" class="cursor-pointer" disabled={update.loading}>Save</Button>
				</form>
				{#if update.error}
					<p class="text-destructive text-xs">{update.error}</p>
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
			{#if usersForbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to list users (TBD: add users by ID)</p>
			{:else}
				<div class="flex flex-row items-center gap-2">
					<Select.Root type="single" bind:value={player}>
						<Select.Trigger class="w-72 cursor-pointer">
							{users.find((user) => user.id === player)?.username ?? 'Select a player'}
						</Select.Trigger>
						<Select.Content>
							{#each users as user (user.id)}
								<Select.Item value={user.id} label={user.username}>{user.username}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
					<Button
						variant="outline"
						class="cursor-pointer"
						disabled={update.loading || !player || Boolean(players[player])}
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
			{/if}
		</div>

		<Separator />

		<div class="flex flex-col gap-2">
			<Card.Title>Danger Zone</Card.Title>
			{#if remove.forbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to delete this team.</p>
			{:else}
				<div class="flex flex-row items-center gap-2">
					{#if confirmDelete}
						<Button
							variant="destructive"
							class="cursor-pointer"
							disabled={remove.loading}
							onclick={() =>
								Submit(async () => {
									await Glue.team.delete(create(DeleteRequestSchema, { gameId: team.gameId, id: team.id }));
									refresh();
								}, setter(remove))}
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
				{#if remove.error}
					<Alert.Root variant="destructive">
						<AlertCircleIcon />
						<Alert.Title>Failed to delete team</Alert.Title>
						<Alert.Description>{remove.error}</Alert.Description>
					</Alert.Root>
				{/if}
			{/if}
		</div>
	</Card.Content>
</Card.Root>
