<script lang="ts">
	import * as Sidebar from '$lib/components/shad/sidebar';
	import * as Collapsible from '$lib/components/shad/collapsible';
	import type { sidebarGroup } from '$lib/components/custom/sidebar/sidebarOptions';
	import OptionRenderer from '$lib/components/custom/sidebar/options/OptionRenderer.svelte';
	import { ChevronDownIcon } from '@lucide/svelte';

	let { group }: { group: sidebarGroup } = $props();

	let open = $state(true);
</script>

{#snippet options()}
	<Sidebar.MenuSub>
		{#each group.contents as item (item.link)}
			<OptionRenderer option={item} />
		{/each}
	</Sidebar.MenuSub>
{/snippet}

<Sidebar.Menu>
	{#if group.collapsible}
		<Collapsible.Root bind:open class="group/collapsible">
			<Sidebar.MenuItem>
				<Collapsible.Trigger>
					{#snippet child({ props })}
						{@const Icon = group.icon}
						<Sidebar.MenuButton {...props}>
							<Icon class="mr-2 h-4 w-4" />
							{group.title}
							<ChevronDownIcon
								class="ml-auto h-4 w-4 {open ? 'rotate-0' : '-rotate-180'} transition-all duration-250 ease-in-out"
							/>
						</Sidebar.MenuButton>
					{/snippet}
				</Collapsible.Trigger>
				<Collapsible.Content>
					{@render options()}
				</Collapsible.Content>
			</Sidebar.MenuItem>
		</Collapsible.Root>
	{:else}
		<Sidebar.MenuItem>
			<Sidebar.MenuButton>
				{@const Icon = group.icon}
				<Icon class="mr-2 h-4 w-4" />
				<span>{group.title}</span>
			</Sidebar.MenuButton>
			{@render options()}
		</Sidebar.MenuItem>
	{/if}
</Sidebar.Menu>
