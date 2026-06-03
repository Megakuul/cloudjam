<script lang="ts">
	import { Glue, Submit } from '$lib';
	import { AggregateRequestsRequestSchema } from '$lib/sdk/v1/admin/system/system_pb';
	import { create } from '@bufbuild/protobuf';
	import { onMount } from 'svelte';
	import { Area, AreaChart, LinearGradient } from 'layerchart';

	let error = $state('');
	let loading = $state(false);

	let windows: { date: Date; value: number }[] = $state([]);

	onMount(() => {
		Submit(
			async () => {
				const resp = await Glue.system.aggregateRequests(
					create(AggregateRequestsRequestSchema, {})
				);
				windows = [];
				for (const requestWindow of resp.requestWindows) {
					windows.push({
						date: new Date(Number(requestWindow.start?.seconds) * 1000),
						value: Number(requestWindow.latency / requestWindow.count) / 1_000_000_000
					});
				}
				windows.sort((a, b) => a.date.getTime() - b.date.getTime());
				console.log(windows.length);
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
		<AreaChart
			data={windows}
			x="date"
			y="value"
			renderContext="svg"
			series={[{ key: 'value', color: 'var(--color-primary)' }]}
			props={{
				xAxis: { format: undefined, tweened: { duration: 200 } },
				tooltip: { item: { format: undefined } }
			}}
		/>
	</div>
	{#if error}
		<div class="p-4 rounded-lg border-red-900 border-[0.1rem] bg-red-600/20">{error}</div>
	{/if}
</div>
