<script lang="ts">
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import * as Card from '$lib/components/ui/card/index.js';
	import * as Table from '$lib/components/ui/table/index.js';
	import * as Alert from '$lib/components/ui/alert';
	import { Glue, Submit } from '$lib';
	import { create } from '@bufbuild/protobuf';
	import { ScanLogsRequestSchema, type Log } from '$lib/sdk/v1/admin/system/system_pb';
	import { timestampFromDate, timestampDate } from '@bufbuild/protobuf/wkt';
	import Input from '$lib/components/ui/input/input.svelte';
	import Button from '$lib/components/ui/button/button.svelte';
	import Badge from '$lib/components/ui/badge/badge.svelte';

	let {
		from
	}: {
		from: number;
	} = $props();

	let error = $state('');

	let logs: Log[] = $state([]);
	let system: string = $state('');
	let procedure: string = $state('');

	$effect(() => {
		Submit(
			async () => {
				const resp = await Glue.system.scanLogs(
					create(ScanLogsRequestSchema, {
						from: timestampFromDate(new Date(Date.now() - Number(from) * 60 * 60 * 1000)),
						to: timestampFromDate(new Date(Date.now())),
						system: system,
						procedure: procedure,
						limit: 20
					})
				);
				logs = resp.logs;
			},
			(e, _) => (error = e)
		);
	});
</script>

<Card.Root class="w-full">
	<Card.Header class="flex gap-2 items-center py-5 space-y-0 border-b sm:flex-row">
		<div class="grid flex-1 gap-1 text-center sm:text-start">
			<Card.Title>System Logs</Card.Title>
			<Card.Description>Analyze CloudJam Logs</Card.Description>
		</div>
	</Card.Header>
	<Card.Content>
		{#if system}
			<Badge variant="secondary">{system}</Badge>
		{/if}
		{#if procedure}
			<Badge variant="secondary">{procedure}</Badge>
		{/if}
	</Card.Content>
</Card.Root>

<Table.Root class="w-full table-fixed">
	<Table.Caption>Requests</Table.Caption>
	<Table.Header>
		<Table.Row>
			<Table.Head class="w-[80px]">Time</Table.Head>
			<Table.Head class="w-[100px]">Level</Table.Head>
			<Table.Head class="w-[200px]">System</Table.Head>
			<Table.Head class="w-[400px]">Action</Table.Head>
			<Table.Head class="w-full">Message</Table.Head>
		</Table.Row>
	</Table.Header>
	<Table.Body>
		{#each logs as log (log)}
			<Table.Row class="overflow-hidden">
				<Table.Cell class="text-slate-100/70">
					{timestampDate(log.timestamp!).toLocaleTimeString('en-US', {
						hour: '2-digit',
						minute: '2-digit',
						hour12: false
					})}
				</Table.Cell>
				<Table.Cell class="font-bold">
					{#if log.level === 'DEBUG'}
						<span class="text-slate-400/90">DBG</span>
					{:else if log.level === 'INFO'}
						<span class="text-emerald-700/90">INF</span>
					{:else if log.level === 'WARN'}
						<span class="text-amber-600/90">WRN</span>
					{:else if log.level === 'ERROR'}
						<span class="text-red-800/90">ERR</span>
					{:else}
						{log.level}
					{/if}
				</Table.Cell>
				<Table.Cell>
					<Button class="w-full" variant="outline" onclick={() => (system = system === log.system ? '' : log.system)}>
						{log.system || '-'}
					</Button>
				</Table.Cell>
				<Table.Cell>
					<Button
						class="w-full"
						variant="outline"
						onclick={() => (procedure = procedure === log.procedure ? '' : log.procedure)}
					>
						{log.procedure || '-'}
					</Button>
				</Table.Cell>
				<Table.Cell>
					<Input readonly={true} value={log.message} />
				</Table.Cell>
			</Table.Row>
		{/each}
	</Table.Body>
</Table.Root>

{#if error}
	<Alert.Root>
		<AlertCircleIcon />
		<Alert.Title>Failed to load log data</Alert.Title>
		<Alert.Description>{error}</Alert.Description>
	</Alert.Root>
{/if}
