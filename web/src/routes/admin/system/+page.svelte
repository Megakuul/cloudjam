<script lang="ts">
	import { Glue } from '$lib';
	import {
		AggregateHitsRequestSchema,
		AggregateLatencyRequestSchema,
		AggregateWindow
	} from '$lib/sdk/v1/admin/system/system_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampFromDate } from '@bufbuild/protobuf/wkt';
	import StatChart from './StatChart.svelte';
	import * as Chart from '$lib/components/ui/chart/index.js';
	import * as Select from '$lib/components/ui/select/index.js';

	// safe generates "layerchart" safe identifiers / keys.
	const safe = (k: string) => k.replace(/[^a-zA-Z0-9_]/g, '_');

	let from = $state('24'); // in hours
	const fromLabel = (from: string) => {
		switch (from) {
			case String(8):
				return 'Last 12 hours';
			case String(24):
				return 'Last 24 hours';
			case String(168):
				return 'Last 7 days';
			case String(720):
				return 'Last 3 months';
		}
	};
	const fromTimeformat = (from: string) => {
		switch (from) {
			case String(8):
			case String(24):
				return (v: any) => {
					return v.toLocaleTimeString('en-US', {
						hour: '2-digit',
						minute: '2-digit',
						hour12: false
					});
				};
			case String(168):
			case String(720):
				return (v: any) => {
					return v.toLocaleDateString('en-US', {
						month: '2-digit',
						day: '2-digit'
					});
				};
		}
	};
</script>

<svelte:head>
	<title>System | CloudJam</title>
	<meta property="og:title" content="System | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="favicon.png" />
</svelte:head>

<div class="flex flex-col gap-4 justify-center items-center w-full">
	<Select.Root type="single" bind:value={from}>
		<Select.Trigger class="w-40 rounded-lg sm:ms-auto" aria-label="Select a value">
			{fromLabel(from)}
		</Select.Trigger>
		<Select.Content class="rounded-xl">
			<Select.Item value={String(8)} class="rounded-lg">{fromLabel(String(8))}</Select.Item>
			<Select.Item value={String(24)} class="rounded-lg">{fromLabel(String(24))}</Select.Item>
			<Select.Item value={String(168)} class="rounded-lg">{fromLabel(String(168))}</Select.Item>
			<Select.Item value={String(720)} class="rounded-lg">{fromLabel(String(720))}</Select.Item>
		</Select.Content>
	</Select.Root>

	<StatChart
		title="Requests"
		description="Endpoint requests of CloudJam service endpoints"
		timeFormat={fromTimeformat(from)}
		load={async () => {
			const resp = await Glue.system.aggregateHits(
				create(AggregateHitsRequestSchema, {
					window: AggregateWindow.Hour,
					from: timestampFromDate(new Date(Date.now() - Number(from) * 60 * 60 * 1000)),
					to: timestampFromDate(new Date(Date.now()))
				})
			);
			const endpoints = Object.keys(resp.endpoints);
			const records: Record<number, any> = {};
			for (let i = 0; i < Number(from); i++) {
				const timestamp = new Date(Date.now() - i * 60 * 60 * 1000);
				timestamp.setMinutes(0, 0, 0);
				records[timestamp.getTime()] = {
					date: timestamp
				};
				for (const endpoint of endpoints) {
					records[timestamp.getTime()][safe(endpoint)] = 0;
				}
			}

			for (const [endpoint, series] of Object.entries(resp.endpoints)) {
				for (let i = 0; i < series.time.length; i++) {
					const timestamp = Number(series.time[i].seconds) * 1000;
					if (records[timestamp]) {
						records[timestamp][safe(endpoint)] = Number(series.count[i]);
					}
				}
			}
			const labels: Chart.ChartConfig = {};
			for (let i = 0; i < endpoints.length; i++) {
				labels[safe(endpoints[i])] = {
					label: endpoints[i],
					color: `var(--chart-${(i % 5) + 1})`
				};
			}
			const data = Object.values(records).sort((a, b) => a.date - b.date);
			return [labels, data];
		}}
	/>
	<StatChart
		title="Latency"
		description="Latency of CloudJam service endpoints"
		timeFormat={fromTimeformat(from)}
		load={async () => {
			const resp = await Glue.system.aggregateLatency(
				create(AggregateLatencyRequestSchema, {
					window: AggregateWindow.Hour,
					from: timestampFromDate(new Date(Date.now() - Number(from) * 60 * 60 * 1000)),
					to: timestampFromDate(new Date(Date.now()))
				})
			);
			const endpoints = Object.keys(resp.endpoints);
			const records: Record<number, any> = {};
			for (let i = 0; i < Number(from); i++) {
				const timestamp = new Date(Date.now() - i * 60 * 60 * 1000);
				timestamp.setMinutes(0, 0, 0);
				records[timestamp.getTime()] = {
					date: timestamp
				};
				for (const endpoint of endpoints) {
					records[timestamp.getTime()][safe(endpoint)] = 0;
				}
			}

			for (const [endpoint, series] of Object.entries(resp.endpoints)) {
				for (let i = 0; i < series.time.length; i++) {
					const timestamp = Number(series.time[i].seconds) * 1000;
					if (records[timestamp]) {
						records[timestamp][safe(endpoint)] = Number(series.latency[i]) / 100_000;
					}
				}
			}
			const labels: Chart.ChartConfig = {};
			for (let i = 0; i < endpoints.length; i++) {
				labels[safe(endpoints[i])] = {
					label: endpoints[i],
					color: `var(--chart-${(i % 5) + 1})`
				};
			}
			const data = Object.values(records).sort((a, b) => a.date - b.date);
			return [labels, data];
		}}
	/>
</div>
