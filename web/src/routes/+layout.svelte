<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { afterNavigate, goto } from '$app/navigation';
	import { getEmail, getPubId, Glue, setToken } from '$lib';
	import { onMount } from 'svelte';
	import { ModeWatcher } from 'mode-watcher';
	import * as Sidebar from '$lib/components/shad/sidebar';
	import AppSidebar from '$lib/components/custom/sidebar/AppSidebar.svelte';

	let { children } = $props();

	let pubId = $state('');
	let email = $state('');

	onMount(() => {
		pubId = getPubId();
		email = getEmail();
	});

	afterNavigate(() => {
		pubId = getPubId();
		email = getEmail();
	});
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

<ModeWatcher />

<Sidebar.Provider>
	<AppSidebar />
	<span class="absolute md:hidden">
		<Sidebar.Trigger />
	</span>

	<main class="h-dvh min-w-0 flex-1">
		<section class="h-full min-w-0 p-3 sm:p-5">
			{@render children?.()}
		</section>
	</main>
</Sidebar.Provider>