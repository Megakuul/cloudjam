<script lang="ts">
	import * as Card from '$lib/components/ui/card/index.js';
	import * as Table from '$lib/components/ui/table/index.js';
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
	<div
		class="flex flex-col justify-center p-3 w-full rounded-xl border border-red-900/90 bg-red-800/20 text-slate-100/80"
	>
		<h1 class="flex flex-row gap-2 items-center text-xl">
			<!-- prettier-ignore -->
			<svg xmlns="http://www.w3.org/2000/svg" width="1em" height="1em" viewBox="0 0 24 24"> <path d="M0 0h24v24H0z" fill="none" /> <g fill="none"> <path fill="currentColor" fill-opacity=".16" d="M3.23 7.913L7.91 3.23c.15-.15.35-.23.57-.23h7.05c.21 0 .42.08.57.23l4.67 4.673c.15.15.23.35.23.57v7.054c0 .21-.08.42-.23.57L16.1 20.77c-.15.15-.35.23-.57.23H8.47a.8.8 0 0 1-.57-.23l-4.67-4.673a.8.8 0 0 1-.23-.57V8.473c0-.21.08-.42.23-.57z" /> <path stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-miterlimit="10" stroke-width="1.5" d="M12 16h.008M12 8v5M3.23 7.913L7.91 3.23c.15-.15.35-.23.57-.23h7.05c.21 0 .42.08.57.23l4.67 4.673c.15.15.23.35.23.57v7.054c0 .21-.08.42-.23.57L16.1 20.77c-.15.15-.35.23-.57.23H8.47a.8.8 0 0 1-.57-.23l-4.67-4.673a.8.8 0 0 1-.23-.57V8.473c0-.21.08-.42.23-.57z" /> </g> </svg>
			<span class="font-bold">Error</span>
		</h1>
		<p class="text-sm">{error}</p>
	</div>
{/if}
