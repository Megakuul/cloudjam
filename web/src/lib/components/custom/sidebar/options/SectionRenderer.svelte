<script lang="ts">
	import * as Sidebar from '$lib/components/shad/sidebar';
	import type { sidebarSection } from '$lib/components/custom/sidebar/sidebarOptions';
	import GroupingRenderer from '$lib/components/custom/sidebar/options/GroupingRenderer.svelte';
	import OptionRenderer from '$lib/components/custom/sidebar/options/OptionRenderer.svelte';

	let { section }: { section: sidebarSection } = $props();
</script>

<Sidebar.Group id={section.title}>
	<Sidebar.GroupLabel>
		{@const Icon = section.icon}
		<Icon class="mr-2 h-4 w-4" />
		{section.title}
	</Sidebar.GroupLabel>
	<Sidebar.GroupContent>
		{#each section.items as item (item.title)}
			{#if 'contents' in item}
				<GroupingRenderer group={item} />
			{:else}
				<OptionRenderer option={item} />
			{/if}
		{/each}
	</Sidebar.GroupContent>
</Sidebar.Group>
