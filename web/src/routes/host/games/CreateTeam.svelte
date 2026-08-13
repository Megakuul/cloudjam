<script lang="ts">
	import { Glue, Submit } from '$lib';
	import ScopeInput from '$lib/components/custom/ScopeInput.svelte';
	import * as Alert from '$lib/components/shad/alert';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import { CreateRequestSchema } from '$lib/sdk/v1/play/team/team_pb';
	import { TeamSchema } from '$lib/sdk/v1/play/team_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';

	let { gameId, scope, oncreated }: { gameId: string; scope: string; oncreated: () => void } = $props();

	let error = $state('');
	let loading = $state(false);
	let forbidden = $state(false);

	// the id is generated client side, players are attached in the team panel.
	// svelte-ignore state_referenced_locally
	let init = $state(create(TeamSchema, { id: crypto.randomUUID(), scope: scope }));

	let request = $derived(create(CreateRequestSchema, { init: { ...init, gameId: gameId } }));
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title>Create Team</Card.Title>
		<Card.Description>Creates a team of the selected game. Players are attached afterwards.</Card.Description>
	</Card.Header>
	<Card.Content>
		<form
			class="flex flex-col gap-4"
			onsubmit={() =>
				Submit(
					async () => {
						await Glue.team.create(request);
						init = create(TeamSchema, { id: crypto.randomUUID(), scope: scope });
						oncreated();
					},
					(e, l, f) => ((error = e), (loading = l), (forbidden = f))
				)}
		>
			<div class="grid gap-4 md:grid-cols-2">
				<div class="flex flex-col gap-1">
					<label for="create-name" class="text-sm">Name</label>
					<Input id="create-name" bind:value={init.name} placeholder="Name of the team" />
					<p class="text-xs text-destructive">{Glue.Validate(TeamSchema, init).violation.name ?? ''}</p>
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-scope" class="text-sm">Scope</label>
					<ScopeInput
						id="create-scope"
						bind:value={init.scope}
						scopes={[scope]}
						placeholder="Scope the team is placed in"
					/>
					<p class="text-xs text-muted-foreground">Defaults to the scope of the game.</p>
				</div>
			</div>
			<Button
				type="submit"
				class="cursor-pointer self-start"
				disabled={loading || Boolean(Glue.Validate(CreateRequestSchema, request).error)}
			>
				Create
			</Button>
			{#if error}
				<Alert.Root variant="destructive">
					<AlertCircleIcon />
					<Alert.Title>{forbidden ? 'Permission denied' : 'Failed to create team'}</Alert.Title>
					<Alert.Description>{error}</Alert.Description>
				</Alert.Root>
			{/if}
		</form>
	</Card.Content>
</Card.Root>
