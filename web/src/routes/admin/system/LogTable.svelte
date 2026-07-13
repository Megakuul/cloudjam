<script lang="ts">
	import * as Chart from '$lib/components/ui/chart/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { scaleUtc } from 'd3-scale';
	import { Area, AreaChart, ChartClipPath } from 'layerchart';
	import { curveNatural } from 'd3-shape';
	import { cubicInOut } from 'svelte/easing';
	import ChartContainer from '$lib/components/ui/chart/chart-container.svelte';
	import * as Table from '$lib/components/ui/table/index.js';
	import { Submit } from '$lib';

	let {
		title,
		description,
		load
	}: {
		title: string;
		description: string;
		load: () => Promise<[Chart.ChartConfig, any[]]>;
	} = $props();

	let error = $state('');

	let data: any[] = $state([]);
	let labels: Chart.ChartConfig = $state({});

	$effect(() => {
		Submit(
			async () => {
				[labels, data] = await load();
			},
			(e, _) => (error = e)
		);
	});
</script>

<Table.Root>
	<Table.Caption>Requests</Table.Caption>
	<Table.Header>
		<Table.Row>
			<Table.Head class="w-[100px]">Invoice</Table.Head>
			<Table.Head>Status</Table.Head>
			<Table.Head>Method</Table.Head>
			<Table.Head class="text-end">Amount</Table.Head>
		</Table.Row>
	</Table.Header>
	<Table.Body>
		{#each invoices as invoice (invoice)}
			<Table.Row>
				<Table.Cell class="font-medium">{invoice.invoice}</Table.Cell>
				<Table.Cell>{invoice.paymentStatus}</Table.Cell>
				<Table.Cell>{invoice.paymentMethod}</Table.Cell>
				<Table.Cell class="text-end">{invoice.totalAmount}</Table.Cell>
			</Table.Row>
		{/each}
	</Table.Body>
	<Table.Footer>
		<Table.Row>
			<Table.Cell colspan={3}>Total</Table.Cell>
			<Table.Cell class="text-end">$2,500.00</Table.Cell>
		</Table.Row>
	</Table.Footer>
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
