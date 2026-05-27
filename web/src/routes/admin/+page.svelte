<script lang="ts">
	import { Glue, Submit } from '$lib';
	import { ListRequestSchema } from '$lib/sdk/v1/admin/user/user_pb';
	import type { User } from '$lib/sdk/v1/admin/user_pb';
	import { create } from '@bufbuild/protobuf';
	import { onMount } from 'svelte';

	let error = $state('');
	let loading = $state(false);

	let users: User[] = $state([]);

	onMount(() => {
		Submit(
			async () => {
				const resp = await Glue.user.list(create(ListRequestSchema, { limit: 100 }));
				users = resp.users;
			},
			(e, l) => ((error = e), (loading = l))
		);
	});
</script>

<svelte:head>
	<title>Admin | CloudJam</title>
	<meta property="og:title" content="Admin | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="favicon.png" />
</svelte:head>

<div class="flex flex-col gap-4 justify-center items-center w-full">
	{#each users as user}
		<div>{user.username}</div>
	{/each}
	{#if error}
		<div class="p-2 m-2 w-full rounded-sm border-[0.05rem] border-red-600/80 bg-red-600/10">
			{error}
		</div>
	{/if}
</div>
