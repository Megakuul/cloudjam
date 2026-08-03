<script lang="ts">
	import * as Sidebar from '$lib/components/shad/sidebar';
	import ProfileDropdown from '$lib/components/custom/sidebar/footer/ProfileDropdown.svelte';
	import SidebarController from '$lib/components/custom/sidebar/SidebarController.svelte';
	import SectionRenderer from '$lib/components/custom/sidebar/options/SectionRenderer.svelte';
	import { hallOfFameOptions, jamOptions } from './sidebarOptions';
	import GroupingRenderer from './options/GroupingRenderer.svelte';

	let loading = $state(true);

	console.log(jamOptions)
</script>

<Sidebar.Root collapsible="icon">
	<Sidebar.Header>
		<SidebarController />
	</Sidebar.Header>

	<Sidebar.Content>
		<SectionRenderer section={jamOptions} />
		<SectionRenderer section={hallOfFameOptions} />
	</Sidebar.Content>

	<Sidebar.Footer>
<!--		<ProfileDropdown bind:loading />-->
	</Sidebar.Footer>
</Sidebar.Root>
