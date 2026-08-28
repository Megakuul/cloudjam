<script lang="ts">
	import { Glue, Submit, type SubmitState } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import * as Table from '$lib/components/shad/table';
	import { ScanLogsRequestSchema, type Log } from '$lib/sdk/v1/admin/system/system_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampDate, timestampFromDate } from '@bufbuild/protobuf/wkt';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import RefreshIcon from '@lucide/svelte/icons/refresh-cw';

	let {
		from,
		to,
		level,
		challenge,
		system = $bindable(),
		procedure = $bindable(),
		limit
	}: {
		from: Date;
		to: Date;
		level: string;
		challenge: string;
		system: string;
		procedure: string;
		limit: number;
	} = $props();

	const severities: Record<string, { short: string; level: string; row: string }> = {
		DEBUG: { short: 'DBG', level: 'text-sky-500', row: 'bg-sky-500/5 hover:bg-sky-500/10' },
		INFO: { short: 'INF', level: 'text-emerald-500', row: 'bg-emerald-500/5 hover:bg-emerald-500/10' },
		WARN: { short: 'WRN', level: 'text-amber-500', row: 'bg-amber-500/10 hover:bg-amber-500/20' },
		ERROR: { short: 'ERR', level: 'text-red-500', row: 'bg-red-500/10 hover:bg-red-500/20' }
	};

	let scanState: SubmitState = $state({ error: '', loading: false, forbidden: false });

	let logs: Log[] = $state([]);
	let expanded = $state('');
	let nonce = $state(0);

	const key = (log: Log, index: number) => `${index}-${log.timestamp?.seconds ?? 0}`;

	$effect(() => {
		nonce;
		Submit(async () => {
			const resp = await Glue.system.scanLogs(
				create(ScanLogsRequestSchema, {
					from: timestampFromDate(from),
					to: timestampFromDate(to),
					system: system,
					procedure: procedure,
					level: level,
					limit: limit,
					challenge: challenge
				})
			);
			logs = resp.logs;
		}, scanState);
	});
</script>

<Card.Root class="w-full">
	<Card.Header class="flex flex-row items-center gap-2 space-y-0 border-b py-5">
		<div class="grid flex-1 gap-1">
			<Card.Title>
				{#if challenge}
					Challenge Logs ({challenge})
				{:else}
					System Logs
				{/if}
			</Card.Title>
		</div>
		<div class="flex flex-row flex-wrap items-center gap-1">
			{#if level}
				<Badge variant="secondary">{level}</Badge>
			{/if}
			{#if system}
				<Badge variant="secondary" class="cursor-pointer" onclick={() => (system = '')}>system: {system}</Badge>
			{/if}
			{#if procedure}
				<Badge variant="secondary" class="cursor-pointer" onclick={() => (procedure = '')}>
					procedure: {procedure}
				</Badge>
			{/if}
			<Button
				variant="outline"
				size="icon"
				title="Refresh"
				class="cursor-pointer"
				disabled={scanState.loading}
				onclick={() => nonce++}
			>
				<RefreshIcon />
			</Button>
		</div>
	</Card.Header>
	<Card.Content>
		{#if scanState.forbidden}
			<p class="text-muted-foreground text-sm italic">You are not allowed to read the system logs.</p>
		{:else}
			<Table.Root class="w-full table-fixed">
				<Table.Header>
					<Table.Row>
						<Table.Head class="w-8"></Table.Head>
						<Table.Head class="w-37.5">Time</Table.Head>
						<Table.Head class="w-15">Level</Table.Head>
						<Table.Head class="w-32.5">System</Table.Head>
						<Table.Head class="w-60">Procedure</Table.Head>
						<Table.Head>Message</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each logs as log, index (key(log, index))}
						{@const open = expanded === key(log, index)}
						<Table.Row
							class="h-9 cursor-pointer {severities[log.level]?.row ?? ''}"
							onclick={() => (expanded = open ? '' : key(log, index))}
						>
							<Table.Cell class="text-muted-foreground">
								<ChevronRightIcon class="size-4 transition-transform duration-200 {open ? 'rotate-90' : ''}" />
							</Table.Cell>
							<Table.Cell class="truncate font-mono text-xs opacity-70">
								{log.timestamp ? timestampDate(log.timestamp).toLocaleString('en-GB') : ''}
							</Table.Cell>
							<Table.Cell class="font-mono text-xs font-bold {severities[log.level]?.level ?? ''}">
								{severities[log.level]?.short ?? log.level}
							</Table.Cell>
							<Table.Cell class="truncate">
								<button
									type="button"
									class="max-w-full cursor-pointer truncate text-left hover:underline"
									title="Filter by {log.system}"
									onclick={(e) => (e.stopPropagation(), (system = system === log.system ? '' : log.system))}
								>
									{log.system || '-'}
								</button>
							</Table.Cell>
							<Table.Cell class="truncate font-mono text-xs">
								<button
									type="button"
									class="max-w-full cursor-pointer truncate text-left hover:underline"
									title="Filter by {log.procedure}"
									onclick={(e) => (e.stopPropagation(), (procedure = procedure === log.procedure ? '' : log.procedure))}
								>
									{log.procedure || '-'}
								</button>
							</Table.Cell>
							<Table.Cell class="truncate font-mono text-xs">{log.message}</Table.Cell>
						</Table.Row>
						{#if open}
							<Table.Row class="bg-muted/30">
								<Table.Cell colspan={6}>
									<div class="flex flex-col gap-3 p-2">
										<div class="flex flex-row items-center gap-2">
											<span class="text-sm font-medium">Log line</span>
											<Button
												variant="outline"
												size="icon"
												title="Copy log line"
												class="cursor-pointer"
												onclick={() => navigator.clipboard.writeText(JSON.stringify(log, null, 2))}
											>
												<CopyIcon />
											</Button>
										</div>
										<pre class="font-mono text-xs whitespace-pre-wrap">{log.message}</pre>
										<span class="text-sm font-medium">Trace</span>
										<pre
											class="text-muted-foreground overflow-x-auto font-mono text-xs whitespace-pre-wrap">{log.trace ||
												'no trace attached'}</pre>
									</div>
								</Table.Cell>
							</Table.Row>
						{/if}
					{:else}
						<Table.Row>
							<Table.Cell colspan={6}>
								<p class="text-muted-foreground p-4 text-sm italic">
									{scanState.loading ? 'Loading logs…' : 'No logs matched the selected range and filters.'}
								</p>
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		{/if}
	</Card.Content>
</Card.Root>

{#if scanState.error}
	<Alert.Root variant="destructive">
		<AlertCircleIcon />
		<Alert.Title>Failed to load log data</Alert.Title>
		<Alert.Description>{scanState.error}</Alert.Description>
	</Alert.Root>
{/if}
