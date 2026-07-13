<script lang="ts">
	import * as Chart from '$lib/components/ui/chart/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { scaleUtc } from 'd3-scale';
	import { Area, AreaChart, ChartClipPath } from 'layerchart';
	import { curveNatural } from 'd3-shape';
	import { cubicInOut } from 'svelte/easing';
	import ChartContainer from '$lib/components/ui/chart/chart-container.svelte';
	import { Submit } from '$lib';

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
	<Card.Header class="flex gap-2 items-center py-5 space-y-0 border-b sm:flex-row">
		<div class="grid flex-1 gap-1 text-center sm:text-start">
			<Card.Title>{title}</Card.Title>
			<Card.Description>{description}</Card.Description>
		</div>
	</Card.Header>
	<Card.Content>
		<ChartContainer config={labels} class="-ml-3 w-full aspect-auto h-[250px]">
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
