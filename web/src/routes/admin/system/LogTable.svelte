<script lang="ts">
	import { Glue, Submit } from '$lib';
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
		system = $bindable(),
		procedure = $bindable(),
		limit
	}: {
		from: Date;
		to: Date;
		level: string;
		system: string;
		procedure: string;
		limit: number;
	} = $props();

	// severities carry their own color so a scan can be read without decoding labels. the row
	// gets a matching tint, the level itself the saturated variant.
	const severities: Record<string, { short: string; level: string; row: string }> = {
		DEBUG: { short: 'DBG', level: 'text-sky-500', row: 'bg-sky-500/5 hover:bg-sky-500/10' },
		INFO: { short: 'INF', level: 'text-emerald-500', row: 'bg-emerald-500/5 hover:bg-emerald-500/10' },
		WARN: { short: 'WRN', level: 'text-amber-500', row: 'bg-amber-500/10 hover:bg-amber-500/20' },
		ERROR: { short: 'ERR', level: 'text-red-500', row: 'bg-red-500/10 hover:bg-red-500/20' }
	};

	let error = $state('');
	let loading = $state(false);
	let forbidden = $state(false);

	let logs: Log[] = $state([]);
	let expanded = $state('');
	let nonce = $state(0);

	// a log line has no id, the timestamp and message identify it well enough to expand it.
	const key = (log: Log, index: number) => `${index}-${log.timestamp?.seconds ?? 0}`;

	$effect(() => {
		nonce;
		Submit(
			async () => {
				const resp = await Glue.system.scanLogs(
					create(ScanLogsRequestSchema, {
						from: timestampFromDate(from),
						to: timestampFromDate(to),
						system: system,
						procedure: procedure,
						level: level,
						limit: limit
					})
				);
				logs = resp.logs;
			},
			(e, l, f) => ((error = e), (loading = l), (forbidden = f))
		);
	});
</script>

<Card.Root class="w-full">
	<Card.Header class="flex flex-row items-center gap-2 space-y-0 border-b py-5">
		<div class="grid flex-1 gap-1">
			<Card.Title>System Logs</Card.Title>
			<Card.Description>
				{logs.length} lines · click a row to expand it, click a system or procedure to filter
			</Card.Description>
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
				disabled={loading}
				onclick={() => nonce++}
			>
				<RefreshIcon />
			</Button>
		</div>
	</Card.Header>
	<Card.Content>
		{#if forbidden}
			<p class="text-sm text-muted-foreground italic">You are not allowed to read the system logs.</p>
		{:else}
			<Table.Root class="w-full table-fixed">
				<Table.Header>
					<Table.Row>
						<Table.Head class="w-[32px]"></Table.Head>
						<Table.Head class="w-[150px]">Time</Table.Head>
						<Table.Head class="w-[60px]">Level</Table.Head>
						<Table.Head class="w-[130px]">System</Table.Head>
						<Table.Head class="w-[240px]">Procedure</Table.Head>
						<Table.Head>Message</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each logs as log, index (key(log, index))}
						{@const open = expanded === key(log, index)}
						<!-- collapsed a log is exactly one line; opening it reveals the untruncated
						     message and the trace underneath. -->
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
											class="overflow-x-auto font-mono text-xs whitespace-pre-wrap text-muted-foreground">{log.trace ||
												'no trace attached'}</pre>
									</div>
								</Table.Cell>
							</Table.Row>
						{/if}
					{:else}
						<Table.Row>
							<Table.Cell colspan={6}>
								<p class="p-4 text-sm text-muted-foreground italic">
									{loading ? 'Loading logs…' : 'No logs matched the selected range and filters.'}
								</p>
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		{/if}
	</Card.Content>
</Card.Root>

{#if error}
	<Alert.Root variant="destructive">
		<AlertCircleIcon />
		<Alert.Title>Failed to load log data</Alert.Title>
		<Alert.Description>{error}</Alert.Description>
	</Alert.Root>
{/if}
