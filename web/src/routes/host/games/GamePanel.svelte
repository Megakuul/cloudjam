<script lang="ts">
	import { fromLocalInput, Glue, Submit, toLocalInput } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import { Separator } from '$lib/components/shad/separator';
	import { DeleteRequestSchema, UpdateRequestSchema } from '$lib/sdk/v1/play/game/game_pb';
	import type { Game } from '$lib/sdk/v1/play/game_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';

	let { game, refresh, deleted }: { game: Game; refresh: () => void; deleted: () => void } = $props();

	type section = { error: string; loading: boolean; forbidden: boolean };
	const setter = (s: section) => (e: string, l: boolean, f: boolean) => (
		(s.error = e),
		(s.loading = l),
		(s.forbidden = f)
	);

	// the panel is remounted (keyed) per game, capturing the initial values is intended.
	// svelte-ignore state_referenced_locally
	let mod = $state({ ...game });
	// svelte-ignore state_referenced_locally
	let from = $state(toLocalInput(game.from));
	// svelte-ignore state_referenced_locally
	let to = $state(toLocalInput(game.to));
	let confirmDelete = $state(false);

	let update: section = $state({ error: '', loading: false, forbidden: false });
	let remove: section = $state({ error: '', loading: false, forbidden: false });
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
			{#if update.forbidden}
				<p class="text-sm text-muted-foreground italic">You are not allowed to update this game.</p>
			{:else}
				<p class="text-sm text-muted-foreground">Games that already started cannot be changed anymore.</p>
				<form
					class="flex flex-col gap-4"
					onsubmit={() =>
						Submit(async () => {
							await Glue.game.update(
								create(UpdateRequestSchema, {
									mod: { ...mod, from: fromLocalInput(from), to: fromLocalInput(to) }
								})
							);
							refresh();
						}, setter(update))}
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
					<Button type="submit" variant="outline" class="cursor-pointer self-start" disabled={update.loading}>
						Save
					</Button>
				</form>
				{#if update.error}
					<p class="text-xs text-destructive">{update.error}</p>
				{/if}
			{/if}
		</div>

		<Separator />

		<div class="flex flex-col gap-2">
			<Card.Title>Danger Zone</Card.Title>
			{#if remove.forbidden}
				<p class="text-sm text-muted-foreground italic">You are not allowed to delete this game.</p>
			{:else}
				<div class="flex flex-row items-center gap-2">
					{#if confirmDelete}
						<Button
							variant="destructive"
							class="cursor-pointer"
							disabled={remove.loading}
							onclick={() =>
								Submit(async () => {
									await Glue.game.delete(create(DeleteRequestSchema, { id: game.id }));
									deleted();
								}, setter(remove))}
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
				{#if remove.error}
					<Alert.Root variant="destructive">
						<AlertCircleIcon />
						<Alert.Title>Failed to delete game</Alert.Title>
						<Alert.Description>{remove.error}</Alert.Description>
					</Alert.Root>
				{/if}
			{/if}
		</div>
	</Card.Content>
</Card.Root>
