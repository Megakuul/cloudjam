<script lang="ts">
	import { Glue, Submit } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import Input from '$lib/components/shad/input/input.svelte';
	import * as Select from '$lib/components/shad/select';
	import { ListRequestSchema as ListDefinitionRequestSchema } from '$lib/sdk/v1/cloud/definition/definition_pb';
	import { ListRequestSchema as ListProviderRequestSchema } from '$lib/sdk/v1/cloud/provider/provider_pb';
	import type { Definition } from '$lib/sdk/v1/cloud/definition_pb';
	import { CreateRequestSchema } from '$lib/sdk/v1/play/challenge/challenge_pb';
	import { ChallengeSchema, type Challenge } from '$lib/sdk/v1/play/challenge_pb';
	import type { Team } from '$lib/sdk/v1/play/team_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import type { Provider } from '$lib/sdk/v1/cloud/provider_pb';
	import { onMount } from 'svelte';

	let {
		gameId,
		teams,
		scope,
		oncreated
	}: {
		gameId: string;
		teams: Team[];
		scope: string;
		oncreated: () => void;
	} = $props();

	let error = $state('');
	let loading = $state(false);
	let forbidden = $state(false);

	let assignedTeams: string[] = $state([]);
	let blueprint: Challenge = $state(create(ChallengeSchema, { id: crypto.randomUUID() }));

	let providers: Provider[] = $state([]);
	async function loadProviders() {
		try {
			providers = (
				await Glue.provider.list(
					create(ListProviderRequestSchema, {
						limit: 100
					})
				)
			).providers;
		} catch {
			providers = [];
		}
	}

	let definitions: Definition[] = $state([]);
	$effect(() => {
		loadDefinitions(blueprint.definitionProviderId);
	});

	async function loadDefinitions(providerId: string) {
		try {
			definitions = (
				await Glue.definition.list(
					create(ListDefinitionRequestSchema, {
						providerId: providerId,
						limit: 100
					})
				)
			).definitions;
		} catch {
			definitions = [];
		}
	}

	function createChallenges() {
		Submit(
			async () => {
				for (const team of assignedTeams) {
					await Glue.challenge.create(
						create(CreateRequestSchema, {
							init: { ...blueprint, teamId: team, gameId: gameId, scope: scope }
						})
					);
				}
				oncreated();
			},
			(e, l, f) => ((error = e), (loading = l), (forbidden = f))
		);
	}

	onMount(() => loadProviders());
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title>Provision Challenges</Card.Title>
		<Card.Description>
			Rollout a challenge to the specified teams. The details are revealed once the team starts the challenge.
		</Card.Description>
	</Card.Header>
	<Card.Content>
		<form class="flex flex-col gap-4" onsubmit={createChallenges}>
			<div class="grid gap-4 md:grid-cols-2">
				<div class="flex flex-col gap-1">
					<label for="select-provider" class="text-sm">Provider</label>
					{#if providers.length > 0}
						<Select.Root type="single" bind:value={blueprint.definitionProviderId}>
							<Select.Trigger id="select-provider" class="w-full cursor-pointer">
								{providers.find((provider) => provider.id === blueprint.definitionProviderId)?.name ??
									'Select provider'}
							</Select.Trigger>
							<Select.Content>
								{#each providers as provider (provider.id)}
									<Select.Item value={provider.id} label={provider.name}>{provider.name}</Select.Item>
								{/each}
							</Select.Content>
						</Select.Root>
					{:else}
						<Input id="select-provider" bind:value={blueprint.definitionProviderId} placeholder="Definition ID" />
					{/if}
				</div>

				<div class="flex flex-col gap-1">
					<label for="select-definition" class="text-sm">Definition</label>
					{#if definitions.length > 0}
						<Select.Root type="single" bind:value={blueprint.definitionId}>
							<Select.Trigger id="select-definition" class="w-full cursor-pointer">
								{definitions.find((definition) => definition.id === blueprint.definitionId)?.name ??
									'Select definition'}
							</Select.Trigger>
							<Select.Content>
								{#each definitions as definition (definition.id)}
									<Select.Item value={definition.id} label={definition.name}>{definition.name}</Select.Item>
								{/each}
							</Select.Content>
						</Select.Root>
					{:else}
						<Input id="select-definition" bind:value={blueprint.definitionId} placeholder="Definition ID" />
					{/if}
				</div>

				<div class="flex flex-col gap-1">
					<label for="select-teams" class="text-sm">Teams</label>
					{#if teams.length > 0}
						<Select.Root type="multiple" bind:value={assignedTeams}>
							<Select.Trigger id="select-teams" class="w-full cursor-pointer">
								{#if assignedTeams.length < 1}
									Select Teams
								{:else}
									{assignedTeams.map((teamId) => teams.find((team) => team.id === teamId)?.name ?? teamId).join(', ')}
								{/if}
							</Select.Trigger>
							<Select.Content>
								{#each teams as team (team.id)}
									<Select.Item value={team.id} label={team.name}>{team.name}</Select.Item>
								{/each}
							</Select.Content>
						</Select.Root>
					{:else}
						<Input id="select-teams" bind:value={assignedTeams} placeholder="Comma separated team ids" />
					{/if}
				</div>
			</div>
			<Button type="submit" class="cursor-pointer self-start">Rollout</Button>
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
