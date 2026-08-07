<script lang="ts">
	import * as Sidebar from '$lib/components/shad/sidebar';
	import type { sidebarOption } from '$lib/components/custom/sidebar/sidebarOptions';

	let { option }: { option: sidebarOption } = $props();
</script>

<Sidebar.MenuItem class="list-none">
	<Sidebar.MenuButton>
		{#snippet child({ props })}
			{@const Icon = option.icon}
			<a href={option.link} {...props}>
				<Icon class="mr-2 h-4 w-4" />
				<span>{option.title}</span>
			</a>
		{/snippet}
	</Sidebar.MenuButton>
</Sidebar.MenuItem>
