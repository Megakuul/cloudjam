<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { onMount } from 'svelte';
	import { ModeWatcher } from 'mode-watcher';
	import * as Sidebar from '$lib/components/shad/sidebar';
	import AppSidebar from '$lib/components/custom/sidebar/AppSidebar.svelte';
	import { loadScopes } from '$lib/scopes.svelte';
	import { getSubject } from '$lib';
	import { goto } from '$app/navigation';
	import { browser } from '$app/env';
	import { page } from '$app/state';

	let { children } = $props();

	if (browser && !getSubject()) {
		if (page.route.id !== '/login' && page.route.id !== '/register') {
			goto('/login');
		}
	}

	onMount(() => {
		loadScopes();
	});
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

{#if page.route.id !== '/login' && page.route.id !== '/register'}
	<ModeWatcher />

	<Sidebar.Provider>
		<AppSidebar />
		<span class="absolute md:hidden">
			<Sidebar.Trigger />
		</span>

		<main class="flex-1 min-w-0 h-dvh">
			<section class="p-3 min-w-0 h-full sm:p-5">
				{@render children?.()}
			</section>
		</main>
	</Sidebar.Provider>
{:else}
	<main class="flex-1 min-w-0 h-dvh">
		<section class="p-3 min-w-0 h-full sm:p-5">
			{@render children?.()}
		</section>
	</main>
{/if}
