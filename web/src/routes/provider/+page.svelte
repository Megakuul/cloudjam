<script lang="ts">
	import { goto } from '$app/navigation';
	import ProviderList from './ProviderList.svelte';
</script>

<svelte:head>
	<title>Provider | CloudJam</title>
	<meta property="og:title" content="Provider | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<ProviderList open={(id) => goto(`/provider/${id}`)} />
