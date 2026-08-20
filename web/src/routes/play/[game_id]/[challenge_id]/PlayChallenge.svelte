<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Separator } from '$lib/components/shad/separator';
	import Spinner from '$lib/components/shad/spinner/spinner.svelte';
	import * as Table from '$lib/components/shad/table';
	import {
		CredentialsRequestSchema,
		StartRequestSchema,
		UncoverClueRequestSchema
	} from '$lib/sdk/v1/play/challenge/challenge_pb';
	import type { Challenge } from '$lib/sdk/v1/play/challenge_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampDate } from '@bufbuild/protobuf/wkt';
	import { SquareArrowOutUpRightIcon } from '@lucide/svelte';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import LightbulbIcon from '@lucide/svelte/icons/lightbulb';
	import PlayIcon from '@lucide/svelte/icons/play';

	let { challenge, refresh }: { challenge: Challenge; refresh: () => void } = $props();

	let credentials = $state('');

	let parsedCredentials: { env: [string, string][]; url: string } = $derived.by(() => {
		const parsed = JSON.parse(credentials);
		return {
			env: Object.entries(JSON.parse(credentials)).filter(
				([key, value]) => typeof value === 'string' && value && key !== 'URL'
			) as [string, string][],
			url: parsed.URL.toString()
		};
	});

	let startState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let credsState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let clueState: SubmitState = $state({ error: '', loading: false, forbidden: false });
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title class="text-2xl">
			{challenge.title || 'Not started yet'}
			{#if startState.loading}
				<Badge variant="default">
					<Spinner />
					Provisioning Challenge
				</Badge>
			{/if}
		</Card.Title>
		<Card.Description>
			<Badge variant="secondary">
				score: {challenge.scoreEvents.reduce((sum, event) => sum + event.change, 0)}
			</Badge>
			{#if !challenge.title}
				<Badge variant="default">not started yet</Badge>
			{/if}
		</Card.Description>
		<Card.Action class="flex flex-row gap-2">
			<Button
				variant="outline"
				class="cursor-pointer"
				disabled={startState.loading || Boolean(challenge.title)}
				onclick={() =>
					Submit(async () => {
						await Glue.challenge.start(create(StartRequestSchema, { gameId: challenge.gameId, id: challenge.id }));
						refresh();
					}, startState)}
			>
				<PlayIcon /> Start
			</Button>
			<Button
				variant="outline"
				class="cursor-pointer"
				disabled={credsState.loading || !challenge.title}
				onclick={() =>
					Submit(async () => {
						credentials = (
							await Glue.challenge.credentials(
								create(CredentialsRequestSchema, { gameId: challenge.gameId, id: challenge.id })
							)
						).credentials;
					}, credsState)}
			>
				Credentials
			</Button>
		</Card.Action>
	</Card.Header>
	<Card.Content class="flex flex-col gap-6">
		{#if startState.error || credsState.error}
			<Alert.Root variant="destructive">
				<AlertCircleIcon />
				<Alert.Title>Action failed</Alert.Title>
				<Alert.Description>{startState.error || credsState.error}</Alert.Description>
			</Alert.Root>
		{/if}

		{#if credentials}
			<Alert.Root>
				<Alert.Title class="flex flex-row items-center gap-3">
					Account credentials
					<Button
						variant="outline"
						class="cursor-pointer"
						onclick={() =>
							navigator.clipboard.writeText(
								parsedCredentials.env.length
									? parsedCredentials.env.map(([name, value]) => `export ${name}=${value}`).join('\n')
									: credentials
							)}
					>
						<CopyIcon /> Copy as environment
					</Button>
					<Button variant="secondary" class="cursor-pointer" href={parsedCredentials.url}>
						<SquareArrowOutUpRightIcon /> Open AWS Console
					</Button>
				</Alert.Title>
			</Alert.Root>
		{/if}

		{#each challenge.errors as message, index (index)}
			<Alert.Root variant="destructive">
				<AlertCircleIcon />
				<Alert.Title>Challenge reported an error</Alert.Title>
				<Alert.Description class="font-mono text-xs break-all">{message}</Alert.Description>
			</Alert.Root>
		{/each}

		{#if challenge.description.length}
			<div class="flex flex-col gap-2">
				<Card.Title>Briefing</Card.Title>
				{#each challenge.description as paragraph, index (index)}
					<p class="text-muted-foreground text-sm">{paragraph}</p>
				{/each}
			</div>
		{/if}

		{#if Object.keys(challenge.assets).length}
			<Separator />

			<div class="flex flex-col gap-2">
				<Card.Title>Assets</Card.Title>
				{#each Object.entries(challenge.assets) as [name, asset] (name)}
					<div class="flex flex-row items-center gap-2 text-sm">
						<span class="font-medium">{name}</span>
						<span class="text-muted-foreground font-mono text-xs break-all">{asset}</span>
					</div>
				{/each}
			</div>
		{/if}

		{#if Object.keys(challenge.clues).length}
			<Separator />

			<div class="flex flex-col gap-2">
				<Card.Title>Clues</Card.Title>
				<p class="text-muted-foreground text-sm">Uncovering a clue usually costs points.</p>
				<Separator />
				{#each Object.entries(challenge.clues) as [name, text] (name)}
					<div class="flex flex-col justify-center gap-2 text-sm">
						<span class="font-medium">{name}</span>
						{#if text !== '<hidden>'}
							<span class="text-muted-foreground">{text}</span>
						{:else}
							<Button
								variant="outline"
								class="cursor-pointer"
								disabled={clueState.loading}
								onclick={() =>
									Submit(async () => {
										await Glue.challenge.uncoverClue(
											create(UncoverClueRequestSchema, {
												gameId: challenge.gameId,
												id: challenge.id,
												clue: name
											})
										);
										refresh();
									}, clueState)}
							>
								<LightbulbIcon /> Uncover
							</Button>
						{/if}
					</div>
				{/each}
				{#if clueState.error}
					<p class="text-destructive text-xs">{clueState.error}</p>
				{/if}
			</div>
		{/if}

		{#if challenge.scoreEvents.length}
			<Separator />

			<div class="flex flex-col gap-2">
				<Card.Title>Score Events</Card.Title>
				<Table.Root>
					<Table.Header>
						<Table.Row>
							<Table.Head>Time</Table.Head>
							<Table.Head>Event</Table.Head>
							<Table.Head>Change</Table.Head>
						</Table.Row>
					</Table.Header>
					<Table.Body>
						{#each challenge.scoreEvents as event, index (index)}
							<Table.Row>
								<Table.Cell>{event.timestamp ? timestampDate(event.timestamp).toLocaleString() : ''}</Table.Cell>
								<Table.Cell>{event.text}</Table.Cell>
								<Table.Cell>{event.change}</Table.Cell>
							</Table.Row>
						{/each}
					</Table.Body>
				</Table.Root>
			</div>
		{/if}
	</Card.Content>
</Card.Root>
