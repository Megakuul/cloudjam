<script lang="ts">
	import { Glue, Submit } from '$lib';
	import * as Alert from '$lib/components/ui/alert';
	import * as Card from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import ZoomOutIcon from '@lucide/svelte/icons/zoom-out';
	import ResetIcon from '@lucide/svelte/icons/rotate-ccw';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import {
		AggregateHitsRequestSchema,
		AggregateLatencyRequestSchema,
		AggregateWindow,
		AggregateWindowSchema
	} from '$lib/sdk/v1/admin/system/system_pb';
	import { create, enumToJson } from '@bufbuild/protobuf';
	import { timestampFromDate } from '@bufbuild/protobuf/wkt';
	import * as Chart from '$lib/components/ui/chart/index.js';
	import ZoomChart from './ZoomChart.svelte';

	// safe generates "layerchart" safe identifiers / keys.
	const safe = (k: string) => k.replace(/[^a-zA-Z0-9_]/g, '_');

	const initialFrom = new Date(Date.now() - 24 * 60 * 60 * 1000);
	const initialTo = new Date();

	let from = $state(initialFrom);
	let to = $state(clampEnd(AggregateWindow.Hour, initialTo));

	// zoomWindow picks the aggregation window (and its bucket size in ms)
	// based on the zoomed range, so zooming in yields higher resolution data.
	function zoomWindow(from: Date, to: Date): [AggregateWindow, number] {
		const span = to.getTime() - from.getTime();
		if (span <= 6 * 60 * 60 * 1000) return [AggregateWindow.Minute, 60 * 1000];
		if (span <= 7 * 24 * 60 * 60 * 1000) return [AggregateWindow.Hour, 60 * 60 * 1000];
		return [AggregateWindow.Day, 24 * 60 * 60 * 1000];
	}

	let [window, bucket] = $derived(zoomWindow(from, to));

	function zoomTimeformat(from: Date, to: Date) {
		// if > 48h show a per day format
		if (to.getTime() - from.getTime() <= 48 * 60 * 60 * 1000) {
			return (v: any) => {
				return v.toLocaleTimeString('en-US', {
					hour: '2-digit',
					minute: '2-digit',
					hour12: false
				});
			};
		}
		return (v: any) => {
			return v.toLocaleDateString('en-US', {
				month: '2-digit',
				day: '2-digit'
			});
		};
	}

	// clampEnd clamps the to date smoothly to the end of the graph depending on the aggregation window.
	function clampEnd(window: AggregateWindow, to: Date): Date {
		let topClamp = new Date();
		switch (window) {
			case AggregateWindow.Month:
				topClamp.setUTCDate(1);
				topClamp.setUTCHours(0, 0, 0, 0);
				break;
			case AggregateWindow.Day:
				topClamp.setUTCHours(0, 0, 0, 0);
			case AggregateWindow.Hour:
				topClamp.setUTCMinutes(0, 0, 0);
			case AggregateWindow.Minute:
				topClamp.setUTCMinutes(0, 0, 0);
		}
		return new Date(Math.min(to.getTime(), topClamp.getTime()));
	}

	let hitLabels: Chart.ChartConfig = $state({});
	let hitData: any[] = $state([]);
	let totalHits: number = $state(0);
	let latencyLabels: Chart.ChartConfig = $state({});
	let latencyData: any[] = $state([]);
	let overallLatency: number = $state(0);

	let error = $state('');
	let forbidden = $state(false);

	async function loadHits(
		window: AggregateWindow,
		bucket: number,
		from: Date,
		to: Date
	): Promise<[number, Chart.ChartConfig, any[]]> {
		let overallHits = 0;
		const resp = await Glue.system.aggregateHits(
			create(AggregateHitsRequestSchema, {
				window: window,
				from: timestampFromDate(from),
				to: timestampFromDate(to)
			})
		);
		const endpoints = Object.keys(resp.endpoints);
		const records: Record<number, any> = {};
		const start = Math.floor(from.getTime() / bucket) * bucket;
		for (let t = start; t <= to.getTime(); t += bucket) {
			records[t] = { date: new Date(t) };
			for (const endpoint of endpoints) {
				records[t][safe(endpoint)] = 0;
			}
		}

		for (const [endpoint, series] of Object.entries(resp.endpoints)) {
			for (let i = 0; i < series.time.length; i++) {
				overallHits += Number(series.count[i]);
				const timestamp = Math.floor((Number(series.time[i].seconds) * 1000) / bucket) * bucket;
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
		return [overallHits, labels, Object.values(records).sort((a, b) => a.date - b.date)];
	}

	async function loadLatency(
		window: AggregateWindow,
		bucket: number,
		from: Date,
		to: Date
	): Promise<[number, Chart.ChartConfig, any[]]> {
		let overallLatency = 0;
		let overallLatencyCount = 0;
		const resp = await Glue.system.aggregateLatency(
			create(AggregateLatencyRequestSchema, {
				window: window,
				from: timestampFromDate(from),
				to: timestampFromDate(to)
			})
		);
		const endpoints = Object.keys(resp.endpoints);
		const records: Record<number, any> = {};
		const start = Math.floor(from.getTime() / bucket) * bucket;
		for (let t = start; t <= to.getTime(); t += bucket) {
			records[t] = { date: new Date(t) };
			for (const endpoint of endpoints) {
				records[t][safe(endpoint)] = 0;
			}
		}

		for (const [endpoint, series] of Object.entries(resp.endpoints)) {
			for (let i = 0; i < series.time.length; i++) {
				overallLatency += Number(series.latency[i]) / 1_000_000;
				overallLatencyCount++;
				const timestamp = Math.floor((Number(series.time[i].seconds) * 1000) / bucket) * bucket;
				if (records[timestamp]) {
					records[timestamp][safe(endpoint)] = Math.floor(Number(series.latency[i]) / 1_000_000);
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
		return [
			Math.floor(overallLatency / overallLatencyCount),
			labels,
			Object.values(records).sort((a, b) => a.date - b.date)
		];
	}

	$effect(() => {
		Submit(
			async () => {
				[totalHits, hitLabels, hitData] = await loadHits(window, bucket, from, to);
				[overallLatency, latencyLabels, latencyData] = await loadLatency(window, bucket, from, to);
			},
			(e, _, f) => ((error = e), (forbidden = f))
		);
	});
</script>

{#if error}
	<Alert.Root>
		<AlertCircleIcon />
		<Alert.Title>Failed to load chart data</Alert.Title>
		<Alert.Description>{error}</Alert.Description>
	</Alert.Root>
{:else if forbidden}
	<Alert.Root>
		<AlertCircleIcon />
		<Alert.Title>Permission Denied</Alert.Title>
		<Alert.Description>You are not allowed to view this section</Alert.Description>
	</Alert.Root>
{:else}
	<Card.Root class="w-full">
		<Card.Header class="flex items-center justify-start gap-20 space-y-0 border-b py-5 sm:flex-row">
			<div>
				<Card.Title class="text-4xl">
					{totalHits}
				</Card.Title>
				<Card.Description class="text-xl">requests served</Card.Description>
			</div>

			<div>
				<Card.Title class="text-4xl">
					{overallLatency} ms
				</Card.Title>
				<Card.Description class="text-xl">avg latency delivered</Card.Description>
			</div>

			<div class="ml-auto flex items-center gap-2">
				<span class="text-xs text-muted-foreground">
					{from.toLocaleDateString(undefined, { hour: '2-digit', minute: '2-digit' })}
					–
					{to.toLocaleDateString(undefined, { hour: '2-digit', minute: '2-digit' })}
				</span>
				<Button
					variant="outline"
					size="icon"
					title="Zoom out"
					class="cursor-pointer"
					onclick={() => {
						const span = to.getTime() - from.getTime();
						from = new Date(from.getTime() - span / 2);
						to = clampEnd(window, new Date(to.getTime() + span / 2));
					}}
				>
					<ZoomOutIcon />
				</Button>
				<Button
					variant="outline"
					size="icon"
					title="Reset"
					class="cursor-pointer"
					onclick={() => {
						from = initialFrom;
						to = clampEnd(window, new Date());
					}}
				>
					<ResetIcon />
				</Button>
			</div>
		</Card.Header>
		<Card.Content class="flex flex-col gap-8">
			<div>
				<Card.Title>Endpoint Requests</Card.Title>
				<Card.Description>
					Hit count per endpoint in {enumToJson(AggregateWindowSchema, window)?.toString().toLowerCase()}s
				</Card.Description>
			</div>
			<ZoomChart
				title="Requests"
				description="Request count per rpc endpoint"
				timeFormat={zoomTimeformat(from, to)}
				labels={hitLabels}
				data={hitData}
				unit={`requests / ${enumToJson(AggregateWindowSchema, window)?.toString().toLowerCase()}`}
				bind:from
				bind:to
			/>

			<div>
				<Card.Title>Endpoint Latency</Card.Title>
				<Card.Description>Average endpoint latency in milliseconds</Card.Description>
			</div>
			<ZoomChart
				title="Latency"
				description="Average request latency per rpc endpoint"
				timeFormat={zoomTimeformat(from, to)}
				labels={latencyLabels}
				data={latencyData}
				unit="ms"
				bind:from
				bind:to
			/>
		</Card.Content>
	</Card.Root>
{/if}
