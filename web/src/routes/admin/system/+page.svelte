<script lang="ts">
	import Button from '$lib/components/ui/button/button.svelte';
	import * as Card from '$lib/components/ui/card';
	import RequestPage from './RequestPage.svelte';

	let page: 'logging' | 'metrics' = $state('logging');
</script>

<svelte:head>
	<title>System | CloudJam</title>
	<meta property="og:title" content="System | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="favicon.png" />
</svelte:head>

<div class="flex w-full flex-col items-center justify-center gap-4">
	<div class="flex w-full flex-row justify-around gap-2">
		<button
			onclick={() => (page = 'logging')}
			class="w-full cursor-pointer rounded-xl p-8 text-3xl transition-all duration-200 hover:bg-slate-50/5
      {page === 'logging' ? 'bg-slate-50/5' : ''}"
		>
			Logging
		</button>
		<button
			onclick={() => (page = 'metrics')}
			class="w-full cursor-pointer rounded-xl p-8 text-3xl transition-all duration-200 hover:bg-slate-50/5
        {page === 'metrics' ? 'bg-slate-50/5' : ''}"
		>
			Metrics
		</button>
	</div>
	{#if page === 'logging'}{:else if page === 'metrics'}
		<RequestPage />
	{/if}
</div>
