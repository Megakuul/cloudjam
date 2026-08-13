<script lang="ts">
	import { fromLocalInput, Glue, Submit, toLocalInput } from '$lib';
	import ScopeInput from '$lib/components/custom/ScopeInput.svelte';
	import * as Alert from '$lib/components/shad/alert';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import { CreateRequestSchema } from '$lib/sdk/v1/play/game/game_pb';
	import { GameSchema } from '$lib/sdk/v1/play/game_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampFromDate } from '@bufbuild/protobuf/wkt';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';

	let { scopes = [], oncreated }: { scopes?: string[]; oncreated: (id: string) => void } = $props();

	let error = $state('');
	let loading = $state(false);
	let forbidden = $state(false);

	// the id is generated client side, the server stores whatever it is given.
	let init = $state(create(GameSchema, { id: crypto.randomUUID() }));
	// the api demands a start in the past and an end in the future.
	let from = $state(toLocalInput(timestampFromDate(new Date(Date.now() - 60 * 1000))));
	let to = $state(toLocalInput(timestampFromDate(new Date(Date.now() + 24 * 60 * 60 * 1000))));

	let request = $derived(
		create(CreateRequestSchema, { init: { ...init, from: fromLocalInput(from), to: fromLocalInput(to) } })
	);
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title>Schedule Game</Card.Title>
		<Card.Description>
			Creates a gameday. Teams and challenges can only be managed while the game has not ended yet.
		</Card.Description>
	</Card.Header>
	<Card.Content>
		<form
			class="flex flex-col gap-4"
			onsubmit={() =>
				Submit(
					async () => {
						await Glue.game.create(request);
						// the new game is opened right away, nobody has to scan for it.
						oncreated(init.id);
						init = create(GameSchema, { id: crypto.randomUUID() });
					},
					(e, l, f) => ((error = e), (loading = l), (forbidden = f))
				)}
		>
			<div class="grid gap-4 md:grid-cols-2">
				<div class="flex flex-col gap-1">
					<label for="create-name" class="text-sm">Name</label>
					<Input id="create-name" bind:value={init.name} placeholder="Name of the game" />
					<p class="text-xs text-destructive">{Glue.Validate(GameSchema, init).violation.name ?? ''}</p>
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-description" class="text-sm">Description</label>
					<Input id="create-description" bind:value={init.description} placeholder="What the game is about" />
					<p class="text-xs text-destructive">{Glue.Validate(GameSchema, init).violation.description ?? ''}</p>
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-from" class="text-sm">From</label>
					<Input id="create-from" type="datetime-local" bind:value={from} />
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-to" class="text-sm">To</label>
					<Input id="create-to" type="datetime-local" bind:value={to} />
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-scope" class="text-sm">Scope</label>
					<ScopeInput id="create-scope" bind:value={init.scope} {scopes} placeholder="Scope the game is placed in" />
					<p class="text-xs text-muted-foreground">You can only attach a scope you possess yourself.</p>
				</div>
			</div>
			<Button type="submit" class="cursor-pointer self-start" disabled={loading}>Schedule</Button>
			<p class="text-xs text-destructive">{Glue.Validate(CreateRequestSchema, request).error}</p>
			{#if error}
				<Alert.Root variant="destructive">
					<AlertCircleIcon />
					<Alert.Title>{forbidden ? 'Permission denied' : 'Failed to schedule game'}</Alert.Title>
					<Alert.Description>{error}</Alert.Description>
				</Alert.Root>
			{/if}
		</form>
	</Card.Content>
</Card.Root>
