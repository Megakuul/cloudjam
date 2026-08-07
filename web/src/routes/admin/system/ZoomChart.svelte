<script lang="ts">
	import * as Chart from '$lib/components/ui/chart/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { scaleUtc } from 'd3-scale';
	import { Area, AreaChart, ChartClipPath } from 'layerchart';
	import { curveNatural } from 'd3-shape';
	import { cubicInOut } from 'svelte/easing';
	import ChartContainer from '$lib/components/ui/chart/chart-container.svelte';

	let {
		title,
		description,
		timeFormat,
		labels,
		data,
		unit = '',
		from = $bindable(),
		to = $bindable()
	}: {
		title: string;
		description: string;
		timeFormat: any;
		labels: Chart.ChartConfig;
		data: any[];
		unit?: string;
		from?: Date;
		to?: Date;
	} = $props();
</script>

<ChartContainer config={labels} class="-ml-3 aspect-auto h-[250px] w-full">
	<AreaChart
		{data}
		x="date"
		xScale={scaleUtc()}
		xDomain={from && to ? [from, to] : undefined}
		series={Object.entries(labels).map(([key, value]) => ({
			key: key,
			label: value.label,
			color: value.color
		}))}
		seriesLayout="stack"
		brush={{
			axis: 'x',
			zoomOnBrush: false,
			onBrushEnd: ({ brush }) => {
				const [start, end] = brush.x ?? [];
				if (start != null && end != null && +end > +start) {
					from = new Date(+start);
					to = new Date(+end);
				}
				brush.reset();
			}
		}}
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
				class="w-max"
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
			>
				{#snippet formatter({ value, name, item })}
					<div
						style="--color-bg: {item.config?.color ?? item.color}"
						class="w-1 shrink-0 self-stretch rounded-[2px] bg-(--color-bg)"
					></div>
					<div class="flex flex-1 items-center justify-between gap-6 leading-none">
						<span class="whitespace-nowrap text-muted-foreground">{name}</span>
						<span class="font-mono font-medium whitespace-nowrap text-foreground tabular-nums">
							{typeof value === 'number' ? value.toLocaleString() : value}{unit ? ` ${unit}` : ''}
						</span>
					</div>
				{/snippet}
			</Chart.Tooltip>
		{/snippet}
	</AreaChart>
</ChartContainer>
