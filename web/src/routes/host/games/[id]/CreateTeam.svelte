<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import LabelInput from '$lib/components/shad/label-input/label-input.svelte';
	import { CreateRequestSchema } from '$lib/sdk/v1/play/team/team_pb';
	import { TeamSchema } from '$lib/sdk/v1/play/team_pb';
	import { create } from '@bufbuild/protobuf';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';

	let { gameId, oncreated }: { gameId: string; oncreated: () => void } = $props();

	let init = $state(create(TeamSchema, { id: crypto.randomUUID() }));

	let createState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	function createTeam() {
		Submit(async () => {
			await Glue.team.create(create(CreateRequestSchema, { init: { ...init, gameId: gameId } }));
			init.name = '';
			oncreated();
		}, createState);
	}
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title>Create Team</Card.Title>
		<Card.Description>Creates a team of the selected game. Players are attached afterwards.</Card.Description>
	</Card.Header>
	<Card.Content>
		<form class="flex flex-col gap-4" onsubmit={createTeam}>
			<div class="grid gap-4 md:grid-cols-2">
				<LabelInput
					bind:value={init.name}
					label="Name"
					placeholder="Name of the team"
					validation={Glue.Validate(TeamSchema, init).violation.name}
				/>
			</div>
			<Button type="submit" class="cursor-pointer self-start">Create</Button>
			{#if createState.error}
				<Alert.Root variant="destructive">
					<AlertCircleIcon />
					<Alert.Title>{createState.forbidden ? 'Permission denied' : 'Failed to create team'}</Alert.Title>
					<Alert.Description>{createState.error}</Alert.Description>
				</Alert.Root>
			{/if}
		</form>
	</Card.Content>
</Card.Root>
