<script lang="ts">
	import { goto } from '$app/navigation';
	import { Button } from '$lib/components/shad/button';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import CreateRole from './CreateRole.svelte';
</script>

<svelte:head>
	<title>Create Role | CloudJam</title>
	<meta property="og:title" content="Create Role | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col gap-4">
	<div class="flex flex-row items-center gap-2">
		<Button variant="ghost" size="icon" class="cursor-pointer" href="/admin/roles/">
			<ChevronLeftIcon />
		</Button>
		<h1 class="text-3xl opacity-80">Create Role</h1>
	</div>

	<CreateRole oncreated={() => goto('/admin/roles/')} />
</div>
