<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import ProviderList from './ProviderList.svelte';
	import ProviderWorkspace from './ProviderWorkspace.svelte';

	// a provider is a root object; accounts and definitions live inside it and are only shown
	// once a provider is opened. the id in the url keeps a provider reachable by link.
	let providerId = $state('');

	function open(id: string) {
		providerId = id;
		goto(id ? `/provider/?id=${encodeURIComponent(id)}` : '/provider/', { keepFocus: true, noScroll: true });
	}

	onMount(() => (providerId = page.url.searchParams.get('id') ?? ''));
</script>

<svelte:head>
	<title>Provider | CloudJam</title>
	<meta property="og:title" content="Provider | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

{#if providerId}
	{#key providerId}
		<ProviderWorkspace {providerId} close={() => open('')} />
	{/key}
{:else}
	<ProviderList open={(id) => open(id)} />
{/if}
