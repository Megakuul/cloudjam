<script lang="ts">
	import { Glue, Submit } from '$lib';
	import ProviderSelect from '$lib/components/custom/ProviderSelect.svelte';
	import ScopeInput from '$lib/components/custom/ScopeInput.svelte';
	import * as Alert from '$lib/components/shad/alert';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import * as Select from '$lib/components/shad/select';
	import { ListRequestSchema as ListDefinitionsRequestSchema } from '$lib/sdk/v1/cloud/definition/definition_pb';
	import type { Definition } from '$lib/sdk/v1/cloud/definition_pb';
	import { CreateRequestSchema } from '$lib/sdk/v1/play/challenge/challenge_pb';
	import { ChallengeSchema } from '$lib/sdk/v1/play/challenge_pb';
	import type { Team } from '$lib/sdk/v1/play/team_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';

	let { gameId, teams, scope, oncreated }: { gameId: string; teams: Team[]; scope: string; oncreated: () => void } =
		$props();

	let error = $state('');
	let loading = $state(false);
	let forbidden = $state(false);

	// the id is generated client side, everything else is filled by the plugin once started.
	// svelte-ignore state_referenced_locally
	let init = $state(create(ChallengeSchema, { id: crypto.randomUUID(), scope: scope }));

	let definitions: Definition[] = $state([]);

	let request = $derived(create(CreateRequestSchema, { init: { ...init, gameId: gameId } }));

	function loadDefinitions() {
		definitions = [];
		init.definitionId = '';
		Submit(
			async () => {
				definitions = (
					await Glue.definition.list(
						create(ListDefinitionsRequestSchema, { providerId: init.definitionProviderId, limit: 100 })
					)
				).definitions;
			},
			(e) => (error = e)
		);
	}
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title>Hand Out Challenge</Card.Title>
		<Card.Description>
			Assigns a challenge definition to a team. The plugin fills in title, description and clues once the challenge is
			started.
		</Card.Description>
	</Card.Header>
	<Card.Content>
		<form
			class="flex flex-col gap-4"
			onsubmit={() =>
				Submit(
					async () => {
						await Glue.challenge.create(request);
						init = create(ChallengeSchema, { id: crypto.randomUUID(), scope: scope });
						oncreated();
					},
					(e, l, f) => ((error = e), (loading = l), (forbidden = f))
				)}
		>
			<div class="grid gap-4 md:grid-cols-2">
				<div class="flex flex-col gap-1">
					<label for="create-team" class="text-sm">Team</label>
					<Select.Root type="single" bind:value={init.teamId}>
						<Select.Trigger id="create-team" class="w-full cursor-pointer">
							{teams.find((team) => team.id === init.teamId)?.name ?? 'Select a team'}
						</Select.Trigger>
						<Select.Content>
							{#each teams as team (team.id)}
								<Select.Item value={team.id} label={team.name}>{team.name}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-scope" class="text-sm">Scope</label>
					<ScopeInput
						id="create-scope"
						bind:value={init.scope}
						scopes={[scope]}
						placeholder="Scope the challenge is placed in"
					/>
					<p class="text-xs text-muted-foreground">Defaults to the scope of the game.</p>
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-provider" class="text-sm">Provider</label>
					<ProviderSelect bind:value={init.definitionProviderId} onselect={() => loadDefinitions()} />
					<p class="text-xs text-muted-foreground">The provider storing the challenge plugin.</p>
				</div>
				<div class="flex flex-col gap-1">
					<label for="create-definition" class="text-sm">Definition</label>
					<Select.Root type="single" bind:value={init.definitionId}>
						<Select.Trigger id="create-definition" class="w-full cursor-pointer">
							{definitions.find((definition) => definition.id === init.definitionId)?.name ?? 'Select a definition'}
						</Select.Trigger>
						<Select.Content>
							{#each definitions as definition (definition.id)}
								<Select.Item value={definition.id} label={definition.name}>{definition.name}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</div>
			</div>
			<Button
				type="submit"
				class="cursor-pointer self-start"
				disabled={loading || Boolean(Glue.Validate(CreateRequestSchema, request).error)}
			>
				Hand Out
			</Button>
			{#if error}
				<Alert.Root variant="destructive">
					<AlertCircleIcon />
					<Alert.Title>{forbidden ? 'Permission denied' : 'Failed to hand out challenge'}</Alert.Title>
					<Alert.Description>{error}</Alert.Description>
				</Alert.Root>
			{/if}
		</form>
	</Card.Content>
</Card.Root>
