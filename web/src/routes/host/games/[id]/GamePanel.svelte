<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import { Separator } from '$lib/components/shad/separator';
	import { DeleteRequestSchema, UpdateRequestSchema } from '$lib/sdk/v1/play/game/game_pb';
	import type { Game } from '$lib/sdk/v1/play/game_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampDate, timestampFromDate } from '@bufbuild/protobuf/wkt';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';

	let { game, refresh, deleted }: { game: Game; refresh: () => void; deleted: () => void } = $props();

	let mod = $state({ ...game });

	// datetime-local inputs work on the local wall clock, protobuf timestamps on utc instants.
	const localInput = (date: Date) =>
		new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);

	let from = $derived(localInput(game.from ? timestampDate(game.from) : new Date()));
	let to = $derived(localInput(game.to ? timestampDate(game.to) : new Date()));

	let confirmDelete = $state(false);

	let updateState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let removeState: SubmitState = $state({ error: '', loading: false, forbidden: false });
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title class="text-2xl">{game.name}</Card.Title>
		<Card.Description>{game.description}</Card.Description>
		<div class="flex flex-row flex-wrap gap-1">
			<Badge variant="outline">scope: {game.scope}</Badge>
			<Badge variant="outline" class="font-mono">{game.id}</Badge>
		</div>
	</Card.Header>
	<Card.Content class="flex flex-col gap-6">
		<div class="flex flex-col gap-2">
			<Card.Title>Schedule</Card.Title>
			{#if updateState.forbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to update this game.</p>
			{:else}
				<p class="text-muted-foreground text-sm">Games that already started cannot be changed anymore.</p>
				<form
					class="flex flex-col gap-4"
					onsubmit={() =>
						Submit(async () => {
							await Glue.game.update(
								create(UpdateRequestSchema, {
									mod: { ...mod, from: timestampFromDate(new Date(from)), to: timestampFromDate(new Date(to)) }
								})
							);
							refresh();
						}, updateState)}
				>
					<div class="grid gap-4 md:grid-cols-2">
						<div class="flex flex-col gap-1">
							<label for="update-name" class="text-sm">Name</label>
							<Input id="update-name" bind:value={mod.name} placeholder="Name of the game" />
						</div>
						<div class="flex flex-col gap-1">
							<label for="update-description" class="text-sm">Description</label>
							<Input id="update-description" bind:value={mod.description} placeholder="What the game is about" />
						</div>
						<div class="flex flex-col gap-1">
							<label for="update-from" class="text-sm">From</label>
							<Input id="update-from" type="datetime-local" bind:value={from} />
						</div>
						<div class="flex flex-col gap-1">
							<label for="update-to" class="text-sm">To</label>
							<Input id="update-to" type="datetime-local" bind:value={to} />
						</div>
					</div>
					<Button type="submit" variant="outline" class="cursor-pointer self-start" disabled={updateState.loading}>
						Save
					</Button>
				</form>
				{#if updateState.error}
					<p class="text-destructive text-xs">{updateState.error}</p>
				{/if}
			{/if}
		</div>

		<Separator />

		<div class="flex flex-col gap-2">
			<Card.Title>Danger Zone</Card.Title>
			{#if removeState.forbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to delete this game.</p>
			{:else}
				<div class="flex flex-row items-center gap-2">
					{#if confirmDelete}
						<Button
							variant="destructive"
							class="cursor-pointer"
							disabled={removeState.loading}
							onclick={() =>
								Submit(async () => {
									await Glue.game.delete(create(DeleteRequestSchema, { id: game.id }));
									deleted();
								}, removeState)}
						>
							Yes, delete {game.name}
						</Button>
						<Button variant="outline" class="cursor-pointer" onclick={() => (confirmDelete = false)}>Cancel</Button>
					{:else}
						<Button variant="destructive" class="cursor-pointer" onclick={() => (confirmDelete = true)}>
							Delete Game
						</Button>
					{/if}
				</div>
				{#if removeState.error}
					<Alert.Root variant="destructive">
						<AlertCircleIcon />
						<Alert.Title>Failed to delete game</Alert.Title>
						<Alert.Description>{removeState.error}</Alert.Description>
					</Alert.Root>
				{/if}
			{/if}
		</div>
	</Card.Content>
</Card.Root>
