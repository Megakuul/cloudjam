<script lang="ts">
	import { Glue, Submit } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import * as Chart from '$lib/components/shad/chart/index.js';
	import { Input } from '$lib/components/shad/input';
	import * as Select from '$lib/components/shad/select';
	import {
		AggregateLogsRequestSchema,
		AggregateWindow,
		AggregateWindowSchema
	} from '$lib/sdk/v1/admin/system/system_pb';
	import { create, enumToJson } from '@bufbuild/protobuf';
	import { timestampFromDate } from '@bufbuild/protobuf/wkt';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import ResetIcon from '@lucide/svelte/icons/rotate-ccw';
	import ZoomOutIcon from '@lucide/svelte/icons/zoom-out';
	import LogTable from './LogTable.svelte';
	import ZoomChart from './ZoomChart.svelte';

	// levels are ordered by severity, the table filters on an exact level.
	const levels = [
		{ value: '', label: 'All levels' },
		{ value: 'DEBUG', label: 'Debug' },
		{ value: 'INFO', label: 'Info' },
		{ value: 'WARN', label: 'Warning' },
		{ value: 'ERROR', label: 'Error' }
	];

	const limits = [
		{ value: '20', label: '20 lines' },
		{ value: '50', label: '50 lines' },
		{ value: '100', label: '100 lines' },
		{ value: '200', label: '200 lines' }
	];

	// levelColors keeps the chart in sync with the severity colors used in the table.
	const levelColors: Record<string, string> = {
		DEBUG: 'var(--color-sky-500)',
		INFO: 'var(--color-emerald-500)',
		WARN: 'var(--color-amber-500)',
		ERROR: 'var(--color-red-500)'
	};

	const initialFrom = new Date(Date.now() - 24 * 60 * 60 * 1000);

	let from = $state(initialFrom);
	let to = $state(new Date());

	let level = $state('');
	let system = $state('');
	let procedure = $state('');
	let limit = $state('50');

	let labels: Chart.ChartConfig = $state({});
	let data: any[] = $state([]);
	let counts: Record<string, number> = $state({});

	let error = $state('');
	let forbidden = $state(false);

	// window picks the aggregation window (and its bucket size in ms) from the zoomed
	// range, so zooming in yields higher resolution data.
	function zoomWindow(from: Date, to: Date): [AggregateWindow, number] {
		const span = to.getTime() - from.getTime();
		if (span <= 6 * 60 * 60 * 1000) return [AggregateWindow.Minute, 60 * 1000];
		if (span <= 7 * 24 * 60 * 60 * 1000) return [AggregateWindow.Hour, 60 * 60 * 1000];
		return [AggregateWindow.Day, 24 * 60 * 60 * 1000];
	}

	let [window, bucket] = $derived(zoomWindow(from, to));

	function zoomTimeformat(from: Date, to: Date) {
		if (to.getTime() - from.getTime() <= 48 * 60 * 60 * 1000) {
			return (v: any) => v.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
		}
		return (v: any) => v.toLocaleDateString('en-US', { month: '2-digit', day: '2-digit' });
	}

	async function loadLevels(window: AggregateWindow, bucket: number, from: Date, to: Date) {
		const resp = await Glue.system.aggregateLogs(
			create(AggregateLogsRequestSchema, {
				window: window,
				from: timestampFromDate(from),
				to: timestampFromDate(to)
			})
		);

		const found = Object.keys(resp.levels);
		const records: Record<number, any> = {};
		const start = Math.floor(from.getTime() / bucket) * bucket;
		for (let t = start; t <= to.getTime(); t += bucket) {
			records[t] = { date: new Date(t) };
			for (const level of found) {
				records[t][level] = 0;
			}
		}

		const totals: Record<string, number> = {};
		for (const [level, series] of Object.entries(resp.levels)) {
			totals[level] = 0;
			for (let i = 0; i < series.time.length; i++) {
				totals[level] += Number(series.count[i]);
				const timestamp = Math.floor((Number(series.time[i].seconds) * 1000) / bucket) * bucket;
				if (records[timestamp]) {
					records[timestamp][level] = Number(series.count[i]);
				}
			}
		}

		const config: Chart.ChartConfig = {};
		for (let i = 0; i < found.length; i++) {
			config[found[i]] = {
				label: found[i],
				color: levelColors[found[i]] ?? `var(--chart-${(i % 5) + 1})`
			};
		}
		return [totals, config, Object.values(records).sort((a, b) => a.date - b.date)] as const;
	}

	$effect(() => {
		Submit(
			async () => {
				[counts, labels, data] = await loadLevels(window, bucket, from, to);
			},
			(e, _, f) => ((error = e), (forbidden = f))
		);
	});
</script>

{#if forbidden}
	<Alert.Root>
		<AlertCircleIcon />
		<Alert.Title>Permission Denied</Alert.Title>
		<Alert.Description>You are not allowed to view this section</Alert.Description>
	</Alert.Root>
{:else}
	<Card.Root class="w-full">
		<Card.Header class="flex items-center justify-start gap-12 space-y-0 border-b py-5 sm:flex-row">
			{#each ['ERROR', 'WARN', 'INFO', 'DEBUG'] as name (name)}
				<div>
					<Card.Title class="text-3xl" style="color: {levelColors[name]}">{counts[name] ?? 0}</Card.Title>
					<Card.Description>{name.toLowerCase()}</Card.Description>
				</div>
			{/each}

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
						to = new Date(Math.min(Date.now(), to.getTime() + span / 2));
					}}
				>
					<ZoomOutIcon />
				</Button>
				<Button
					variant="outline"
					size="icon"
					title="Reset"
					class="cursor-pointer"
					onclick={() => ((from = new Date(Date.now() - 24 * 60 * 60 * 1000)), (to = new Date()))}
				>
					<ResetIcon />
				</Button>
			</div>
		</Card.Header>
		<Card.Content class="flex flex-col gap-6">
			<div>
				<Card.Title>Log Volume</Card.Title>
				<Card.Description>
					Log lines per severity in {enumToJson(AggregateWindowSchema, window)?.toString().toLowerCase()}s. Drag on the
					chart to zoom into a range.
				</Card.Description>
			</div>
			<ZoomChart
				title="Logs"
				description="Log lines per severity"
				timeFormat={zoomTimeformat(from, to)}
				{labels}
				{data}
				unit={`lines / ${enumToJson(AggregateWindowSchema, window)?.toString().toLowerCase()}`}
				bind:from
				bind:to
			/>

			<div class="flex flex-row flex-wrap items-end gap-2">
				<div class="flex flex-col gap-1">
					<label for="log-level" class="text-sm">Level</label>
					<Select.Root type="single" bind:value={level}>
						<Select.Trigger id="log-level" class="w-40 cursor-pointer">
							{levels.find((item) => item.value === level)?.label}
						</Select.Trigger>
						<Select.Content>
							{#each levels as item (item.value)}
								<Select.Item value={item.value} label={item.label}>{item.label}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</div>
				<div class="flex flex-col gap-1">
					<label for="log-system" class="text-sm">System</label>
					<Input id="log-system" class="w-56" bind:value={system} placeholder="e.g. server" />
				</div>
				<div class="flex flex-col gap-1">
					<label for="log-procedure" class="text-sm">Procedure</label>
					<Input
						id="log-procedure"
						class="w-72"
						bind:value={procedure}
						placeholder="e.g. /v1.admin.user.UserService/List"
					/>
				</div>
				<div class="flex flex-col gap-1">
					<label for="log-limit" class="text-sm">Lines</label>
					<Select.Root type="single" bind:value={limit}>
						<Select.Trigger id="log-limit" class="w-32 cursor-pointer">
							{limits.find((item) => item.value === limit)?.label}
						</Select.Trigger>
						<Select.Content>
							{#each limits as item (item.value)}
								<Select.Item value={item.value} label={item.label}>{item.label}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</div>
				<Button
					variant="outline"
					class="cursor-pointer"
					disabled={!level && !system && !procedure}
					onclick={() => ((level = ''), (system = ''), (procedure = ''))}
				>
					Clear
				</Button>
			</div>
		</Card.Content>
	</Card.Root>

	<LogTable {from} {to} {level} bind:system bind:procedure limit={Number(limit)} />

	{#if error}
		<Alert.Root variant="destructive">
			<AlertCircleIcon />
			<Alert.Title>Failed to load log data</Alert.Title>
			<Alert.Description>{error}</Alert.Description>
		</Alert.Root>
	{/if}
{/if}
