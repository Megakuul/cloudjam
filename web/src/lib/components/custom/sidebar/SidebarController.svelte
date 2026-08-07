<script lang="ts">
	import * as Sidebar from '$lib/components/shad/sidebar';
	import * as Kbd from '$lib/components/shad/kbd';
	import { useSidebar } from '$lib/components/shad/sidebar';
	import { isMacOS } from './';

	const sidebar = useSidebar();
</script>

{#if !sidebar.isMobile}
	<span class="ml-auto inline-flex items-center">
		{#if sidebar.open}
			<Kbd.Group>
				{#if isMacOS()}
					<Kbd.Root>⌘</Kbd.Root>
				{:else}
					<Kbd.Root>Ctrl</Kbd.Root>
				{/if}
				<span>+</span>
				<Kbd.Root>B</Kbd.Root>
			</Kbd.Group>
		{/if}
		<Sidebar.Trigger />
	</span>
{/if}
