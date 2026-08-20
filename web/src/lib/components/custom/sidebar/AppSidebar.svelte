<script lang="ts">
	import * as Sidebar from '$lib/components/shad/sidebar';
	import ProfileDropdown from '$lib/components/custom/sidebar/footer/ProfileDropdown.svelte';
	import SidebarController from '$lib/components/custom/sidebar/SidebarController.svelte';
	import SectionRenderer from '$lib/components/custom/sidebar/options/SectionRenderer.svelte';
	import { adminOptions, hallOfFameOptions, hostOptions, jamOptions, providerOptions } from './sidebarOptions';
	import { onMount } from 'svelte';
	import { afterNavigate } from '$app/navigation';
	import { getEmail, getPubId } from '$lib';

	let loading = $state(true);

	onMount(() => {
		loading = false;
	});

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

<Sidebar.Root collapsible="icon">
	<Sidebar.Header>
		<SidebarController />
	</Sidebar.Header>

	<Sidebar.Content>
		<SectionRenderer section={jamOptions} />
		<SectionRenderer section={hostOptions} />
		<SectionRenderer section={providerOptions} />
		<SectionRenderer section={adminOptions} />
		<!-- <SectionRenderer section={hallOfFameOptions} /> -->
	</Sidebar.Content>

	<Sidebar.Footer>
		<ProfileDropdown bind:loading {pubId} {email} />
	</Sidebar.Footer>
</Sidebar.Root>
