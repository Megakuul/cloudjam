<script lang="ts">
	import { Glue, Submit } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Separator } from '$lib/components/shad/separator';
	import * as Table from '$lib/components/shad/table';
	import {
		CredentialsRequestSchema,
		StartRequestSchema,
		UncoverClueRequestSchema
	} from '$lib/sdk/v1/play/challenge/challenge_pb';
	import type { Challenge } from '$lib/sdk/v1/play/challenge_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampDate } from '@bufbuild/protobuf/wkt';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import LightbulbIcon from '@lucide/svelte/icons/lightbulb';
	import PlayIcon from '@lucide/svelte/icons/play';

	let { challenge, refresh }: { challenge: Challenge; refresh: () => void } = $props();

	type section = { error: string; loading: boolean; forbidden: boolean };
	const setter = (s: section) => (e: string, l: boolean, f: boolean) => (
		(s.error = e),
		(s.loading = l),
		(s.forbidden = f)
	);

	let credentials = $state('');

	// the aws provider hands out credentials as a json map of environment variables
	// (see internal/provider/aws/credentials.go), anything else is shown raw.
	let environment = $derived.by(() => {
		try {
			return Object.entries(JSON.parse(credentials)).filter(([, value]) => typeof value === 'string' && value) as [
				string,
				string
			][];
		} catch {
			return [];
		}
	});

	let start: section = $state({ error: '', loading: false, forbidden: false });
	let creds: section = $state({ error: '', loading: false, forbidden: false });
	let clue: section = $state({ error: '', loading: false, forbidden: false });
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title class="text-2xl">{challenge.title || 'Not started yet'}</Card.Title>
		<Card.Description>
			<Badge variant="secondary">
				score: {challenge.scoreEvents.reduce((sum, event) => sum + event.change, 0)}
			</Badge>
		</Card.Description>
		<Card.Action class="flex flex-row gap-2">
			<Button
				variant="outline"
				class="cursor-pointer"
				disabled={start.loading}
				onclick={() =>
					Submit(async () => {
						await Glue.challenge.start(create(StartRequestSchema, { gameId: challenge.gameId, id: challenge.id }));
						refresh();
					}, setter(start))}
			>
				<PlayIcon /> Start
			</Button>
			<Button
				variant="outline"
				class="cursor-pointer"
				disabled={creds.loading}
				onclick={() =>
					Submit(async () => {
						credentials = (
							await Glue.challenge.credentials(
								create(CredentialsRequestSchema, { gameId: challenge.gameId, id: challenge.id })
							)
						).credentials;
					}, setter(creds))}
			>
				Credentials
			</Button>
		</Card.Action>
	</Card.Header>
	<Card.Content class="flex flex-col gap-6">
		{#if start.error || creds.error}
			<Alert.Root variant="destructive">
				<AlertCircleIcon />
				<Alert.Title>Action failed</Alert.Title>
				<Alert.Description>{start.error || creds.error}</Alert.Description>
			</Alert.Root>
		{/if}

		{#if credentials}
			<Alert.Root>
				<Alert.Title class="flex flex-row items-center gap-2">
					Account credentials
					<Button
						variant="outline"
						size="sm"
						class="cursor-pointer"
						onclick={() =>
							navigator.clipboard.writeText(
								environment.length
									? environment.map(([name, value]) => `export ${name}=${value}`).join('\n')
									: credentials
							)}
					>
						<CopyIcon /> Copy as environment
					</Button>
				</Alert.Title>
				<Alert.Description class="flex flex-col gap-1">
					{#each environment as [name, value] (name)}
						<div class="flex flex-row items-center gap-2 font-mono text-xs break-all">
							<span class="w-56 shrink-0 font-medium">{name}</span>
							<span>{value}</span>
							<Button
								variant="outline"
								size="icon"
								title="Copy {name}"
								class="cursor-pointer"
								onclick={() => navigator.clipboard.writeText(value)}
							>
								<CopyIcon />
							</Button>
						</div>
					{:else}
						<span class="font-mono text-xs break-all">{credentials}</span>
					{/each}
				</Alert.Description>
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
					<p class="text-sm text-muted-foreground">{paragraph}</p>
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
						<span class="font-mono text-xs break-all text-muted-foreground">{asset}</span>
					</div>
				{/each}
			</div>
		{/if}

		{#if Object.keys(challenge.clues).length}
			<Separator />

			<div class="flex flex-col gap-2">
				<Card.Title>Clues</Card.Title>
				<p class="text-sm text-muted-foreground">Uncovering a clue usually costs points.</p>
				{#each Object.entries(challenge.clues) as [name, text] (name)}
					<div class="flex flex-row items-center gap-2 text-sm">
						<span class="font-medium">{name}</span>
						{#if text}
							<span class="text-muted-foreground">{text}</span>
						{:else}
							<Button
								variant="outline"
								size="sm"
								class="cursor-pointer"
								disabled={clue.loading}
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
									}, setter(clue))}
							>
								<LightbulbIcon /> Uncover
							</Button>
						{/if}
					</div>
				{/each}
				{#if clue.error}
					<p class="text-xs text-destructive">{clue.error}</p>
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
