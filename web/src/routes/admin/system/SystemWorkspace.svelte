<script lang="ts">
	import { Button } from '$lib/components/shad/button';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import LogSection from './LogSection.svelte';
	import MetricSection from './MetricSection.svelte';

	let { close }: { close: () => void } = $props();

	const tabs = ['logs', 'metrics', 'cost'] as const;

	let tab: (typeof tabs)[number] = $state('logs');
</script>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<Button variant="ghost" size="icon" class="cursor-pointer" onclick={() => close()}>
			<ChevronLeftIcon />
		</Button>
		<h1 class="text-3xl opacity-80">System</h1>
	</div>

	<div class="flex flex-row gap-2 border-b">
		{#each tabs as name (name)}
			<button
				type="button"
				class="cursor-pointer border-b-2 px-4 py-2 capitalize transition-all duration-200 {tab === name
					? 'border-primary'
					: 'border-transparent opacity-60 hover:opacity-100'}"
				onclick={() => (tab = name)}
			>
				{name}
			</button>
		{/each}
	</div>

	{#if tab === 'logs'}
		<LogSection />
	{:else if tab === 'metrics'}
		<MetricSection />
	{:else if tab === 'cost'}
		<p>TBD (here would be cost / budget analytics aggregated from each providers interface)</p>
	{/if}
</div>
