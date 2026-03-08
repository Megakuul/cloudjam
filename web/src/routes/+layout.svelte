<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import 'carbon-components-svelte/css/all.css';

	let { children } = $props();

	let theme = $state('white');
	$effect(() => {
		document.documentElement.setAttribute('theme', theme);
	});
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>
{@render children()}
