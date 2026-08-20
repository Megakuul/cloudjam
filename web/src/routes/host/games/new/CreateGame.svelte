<script lang="ts">
	import { goto } from '$app/navigation';
	import { Glue, Submit, type SubmitState } from '$lib';
	import OptionalSelect, { type Suggestion } from '$lib/components/custom/OptionalSelect.svelte';
	import * as Alert from '$lib/components/shad/alert';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import LabelInput from '$lib/components/shad/label-input/label-input.svelte';
	import { scopes } from '$lib/scopes.svelte';
	import { CreateRequestSchema } from '$lib/sdk/v1/play/game/game_pb';
	import { GameSchema } from '$lib/sdk/v1/play/game_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampFromDate } from '@bufbuild/protobuf/wkt';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';

	let createState: SubmitState = $state({ loading: false, error: '', forbidden: false });
	let error = $state('');
	let loading = $state(false);
	let forbidden = $state(false);

	let init = $state(create(GameSchema, { id: crypto.randomUUID() }));
	const localInput = (date: Date) =>
		new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);

	let from = $state(localInput(new Date(Date.now() + 1 * 60 * 60 * 1000)));
	let to = $state(localInput(new Date(Date.now() + 3 * 60 * 60 * 1000)));

	let request = $derived(
		create(CreateRequestSchema, {
			init: { ...init, from: timestampFromDate(new Date(from)), to: timestampFromDate(new Date(to)) }
		})
	);
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title>Host Game</Card.Title>
		<Card.Description>Teams and challenges can only be managed while the game has not ended yet.</Card.Description>
	</Card.Header>
	<Card.Content>
		<form
			class="flex flex-col gap-4"
			onsubmit={() =>
				Submit(async () => {
					const resp = await Glue.game.create(request);
					goto(`/host/games/${resp.id}`);
				}, createState)}
		>
			<div class="grid gap-4 md:grid-cols-2">
				<LabelInput
					bind:value={init.name}
					label={'Name'}
					placeholder={'Name of the game'}
					validation={Glue.Validate(GameSchema, init).violation.name}
				/>
				<LabelInput
					bind:value={init.description}
					label={'Description'}
					placeholder={'What the game is about'}
					validation={Glue.Validate(GameSchema, init).violation.description}
				/>
				<label class="text-sm">
					From
					<Input type="datetime-local" bind:value={from} />
				</label>
				<label class="text-sm">
					To
					<Input type="datetime-local" bind:value={to} />
				</label>
				<label class="text-sm">
					Scope
					<OptionalSelect
						bind:value={init.scope}
						placeholder="Scope the game is placed in"
						suggestions={scopes.map((scope) => ({ id: scope, title: scope })) satisfies Suggestion[]}
						class="w-full"
					/>
				</label>
			</div>
			<Button type="submit" class="cursor-pointer self-start" disabled={loading}>Schedule</Button>
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
