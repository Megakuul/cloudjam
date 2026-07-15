<script lang="ts">
	import * as Chart from '$lib/components/ui/chart/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import { scaleUtc } from 'd3-scale';
	import { Area, AreaChart, ChartClipPath } from 'layerchart';
	import { curveNatural } from 'd3-shape';
	import { cubicInOut } from 'svelte/easing';
	import ChartContainer from '$lib/components/ui/chart/chart-container.svelte';
	import { Submit } from '$lib';
	import * as Alert from '$lib/components/ui/alert';

	let {
		title,
		description,
		timeFormat,
		load
	}: {
		title: string;
		description: string;
		timeFormat: any;
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

<Card.Root class="w-full">
	<Card.Header class="flex items-center gap-2 space-y-0 border-b py-5 sm:flex-row">
		<div class="grid flex-1 gap-1 text-center sm:text-start">
			<Card.Title>{title}</Card.Title>
			<Card.Description>{description}</Card.Description>
		</div>
	</Card.Header>
	<Card.Content>
		<ChartContainer config={labels} class="-ml-3 aspect-auto h-[250px] w-full">
			<AreaChart
				legend
				{data}
				x="date"
				xScale={scaleUtc()}
				series={Object.entries(labels).map(([key, value]) => ({
					key: key,
					label: value.label,
					color: value.color
				}))}
				seriesLayout="stack"
				props={{
					xAxis: {
						format: timeFormat
					},
					yAxis: { format: () => '' }
				}}
			>
				{#snippet marks({ context })}
					<ChartClipPath
						initialWidth={0}
						motion={{
							width: { type: 'tween', duration: 1000, easing: cubicInOut }
						}}
					>
						{#each context.series.visibleSeries as s (s.key)}
							<Area
								seriesKey={s.key}
								curve={curveNatural}
								fillOpacity={0.4}
								line={{ class: 'stroke-1' }}
								motion="tween"
								{...s.props}
							/>
						{/each}
					</ChartClipPath>
				{/snippet}
				{#snippet tooltip()}
					<Chart.Tooltip
						labelFormatter={(v: Date) => {
							return v.toLocaleDateString('en-US', {
								month: '2-digit',
								day: '2-digit',
								hour: '2-digit',
								minute: '2-digit',
								hour12: false
							});
						}}
						indicator="line"
					/>
				{/snippet}
			</AreaChart>
		</ChartContainer>
	</Card.Content>
</Card.Root>

{#if error}
	<Alert.Root>
		<AlertCircleIcon />
		<Alert.Title>Failed to load chart data</Alert.Title>
		<Alert.Description>{error}</Alert.Description>
	</Alert.Root>
{/if}
