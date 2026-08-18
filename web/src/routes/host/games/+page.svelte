<script lang="ts">
	import { goto } from '$app/navigation';
	import { Glue, Submit } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import * as Table from '$lib/components/shad/table';
	import { ListRequestSchema } from '$lib/sdk/v1/play/game/game_pb';
	import type { Game } from '$lib/sdk/v1/play/game_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampDate } from '@bufbuild/protobuf/wkt';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import { onMount } from 'svelte';

	const limit = 20;

	let error = $state('');
	let loading = $state(true);
	let forbidden = $state(false);

	let games: Game[] = $state([]);
	let exhausted = $state(true);

	let id = $state('');

	function status(game: Game): string {
		const now = Date.now();
		if (game.from && timestampDate(game.from).getTime() > now) return 'upcoming';
		if (game.to && timestampDate(game.to).getTime() < now) return 'ended';
		return 'running';
	}

	function load(startAfter?: string) {
		Submit(
			async () => {
				const resp = await Glue.game.list(create(ListRequestSchema, { limit: limit, startAfter: startAfter }));
				games = startAfter ? [...games, ...resp.games] : resp.games;
				exhausted = resp.games.length < limit;
			},
			(e, l, f) => ((error = e), (loading = l), (forbidden = f))
		);
	}

	onMount(() => load());
</script>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<Button variant="ghost" size="icon" class="cursor-pointer" href="/host/">
			<ChevronLeftIcon />
		</Button>
		<h1 class="text-3xl opacity-80">Games</h1>
		<Button variant="outline" class="ml-auto cursor-pointer" href="/host/games/new/">
			<PlusIcon /> Host Game
		</Button>
	</div>

	{#if forbidden}
		<Card.Root class="w-full">
			<Card.Header>
				<Card.Title>Open a Game</Card.Title>
				<Card.Description>You are not allowed to list games, open the one you host by its id.</Card.Description>
			</Card.Header>
			<Card.Content>
				<form class="flex flex-row items-center gap-2" onsubmit={() => goto(`/host/games/${id}`)}>
					<Input class="max-w-96" bind:value={id} placeholder="Game id" />
					<Button type="submit" class="cursor-pointer" disabled={!id.trim()}>Open</Button>
				</form>
			</Card.Content>
		</Card.Root>
	{:else}
		<Table.Root>
			<Table.Header>
				<Table.Row>
					<Table.Head>Name</Table.Head>
					<Table.Head>Description</Table.Head>
					<Table.Head>From</Table.Head>
					<Table.Head>To</Table.Head>
					<Table.Head>Status</Table.Head>
					<Table.Head>Scope</Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each games as game (game.id)}
					<Table.Row class="cursor-pointer" onclick={() => goto(`/host/games/${game.id}`)}>
						<Table.Cell class="font-medium">{game.name}</Table.Cell>
						<Table.Cell>{game.description}</Table.Cell>
						<Table.Cell>{game.from ? timestampDate(game.from).toLocaleString() : ''}</Table.Cell>
						<Table.Cell>{game.to ? timestampDate(game.to).toLocaleString() : ''}</Table.Cell>
						<Table.Cell><Badge variant="secondary">{status(game)}</Badge></Table.Cell>
						<Table.Cell><Badge variant="outline">{game.scope}</Badge></Table.Cell>
					</Table.Row>
				{:else}
					<Table.Row>
						<Table.Cell colspan={6}>
							<p class="text-muted-foreground p-4 text-sm italic">
								{loading ? 'Loading games…' : 'No games yet. Schedule one to hand challenges out to teams.'}
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
				disabled={loading}
				onclick={() => load(games.at(-1)?.id)}
			>
				Load More
			</Button>
		{/if}
	{/if}

	{#if error}
		<Alert.Root variant="destructive">
			<AlertCircleIcon />
			<Alert.Title>Failed to load games</Alert.Title>
			<Alert.Description>{error}</Alert.Description>
		</Alert.Root>
	{/if}
</div>
