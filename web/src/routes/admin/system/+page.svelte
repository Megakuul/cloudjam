<script lang="ts">
	import { Glue, Submit } from '$lib';
	import { AggregateRequestsRequestSchema } from '$lib/sdk/v1/admin/system/system_pb';
	import { create } from '@bufbuild/protobuf';
	import { onMount } from 'svelte';
	import { Area, AreaChart, LinearGradient } from 'layerchart';

	let error = $state('');
	let loading = $state(false);

	let windows: { key: string; color: string; data: { date: Date; value: number }[] }[] = $state([]);

	onMount(() => {
		Submit(
			async () => {
				const resp = await Glue.system.aggregateRequests(
					create(AggregateRequestsRequestSchema, {})
				);
				const endpoints: Record<string, { date: Date; value: number }[]> = {};
				for (const requestWindow of resp.requestWindows) {
					endpoints[requestWindow.endpoint] = endpoints[requestWindow.endpoint] ?? [];
					endpoints[requestWindow.endpoint].push({
						date: new Date(Number(requestWindow.start?.seconds) * 1000),
						value: Number(requestWindow.latency / requestWindow.count) / 1_000_000_000
					});
				}
				const getHueFromString = (str: string) => {
					let hash = 0;
					for (let i = 0; i < str.length; i++) {
						hash = str.charCodeAt(i) + ((hash << 5) - hash);
					}
					return Math.abs(hash) % 360;
				};
				windows = Object.entries(endpoints).map(([key, value]) => ({
					key: key,
					color: `hsl(${getHueFromString(key)}, 65%, 55%)`,
					data: value.sort((a, b) => a.date.getTime() - b.date.getTime())
				}));
			},
			(e, l) => ((error = e), (loading = l))
		);
	});
</script>

<svelte:head>
	<title>System | CloudJam</title>
	<meta property="og:title" content="System | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="favicon.png" />
</svelte:head>

<div class="flex flex-col gap-4 justify-center items-center w-full">
	<h1>System</h1>
	<div class="p-4 w-full rounded border h-[300px]">
		<AreaChart x="date" y="value" series={windows} renderContext="svg" />
		<!-- <AreaChart -->
		<!-- 	data={windows} -->
		<!-- 	x="date" -->
		<!-- 	y="value" -->
		<!-- 	renderContext="svg" -->
		<!-- 	series={[{ key: 'value', color: 'var(--color-primary)' }]} -->
		<!-- 	props={{ -->
		<!-- 		xAxis: { format: undefined, tweened: { duration: 200 } }, -->
		<!-- 		tooltip: { item: { format: undefined } } -->
		<!-- 	}} -->
		<!-- /> -->
	</div>
	{#if error}
		<div class="p-4 rounded-lg border-red-900 border-[0.1rem] bg-red-600/20">{error}</div>
	{/if}
</div>
