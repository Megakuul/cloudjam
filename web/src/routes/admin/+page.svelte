<script lang="ts">
	import * as Card from '$lib/components/shad/card';
	import UsersIcon from '@lucide/svelte/icons/users';
	import ShieldCheckIcon from '@lucide/svelte/icons/shield-check';
	import ActivityIcon from '@lucide/svelte/icons/activity';
</script>

<svelte:head>
	<title>Admin | CloudJam</title>
	<meta property="og:title" content="Admin | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col items-center gap-4">
	<h1 class="my-8 text-4xl opacity-80">Administration</h1>
	<div class="grid w-full gap-4 md:grid-cols-3">
		<a href="/admin/users/">
			<Card.Root class="h-full cursor-pointer transition-all duration-200 hover:bg-slate-50/5">
				<Card.Header>
					<UsersIcon class="size-8" />
					<Card.Title class="text-2xl">Users</Card.Title>
					<Card.Description>Invite, manage and scope the users of your hierarchy.</Card.Description>
				</Card.Header>
			</Card.Root>
		</a>
		<a href="/admin/roles/">
			<Card.Root class="h-full cursor-pointer transition-all duration-200 hover:bg-slate-50/5">
				<Card.Header>
					<ShieldCheckIcon class="size-8" />
					<Card.Title class="text-2xl">Roles</Card.Title>
					<Card.Description>Create roles and configure their rbac permission policies.</Card.Description>
				</Card.Header>
			</Card.Root>
		</a>
		<a href="/admin/system/">
			<Card.Root class="h-full cursor-pointer transition-all duration-200 hover:bg-slate-50/5">
				<Card.Header>
					<ActivityIcon class="size-8" />
					<Card.Title class="text-2xl">System</Card.Title>
					<Card.Description>Observe system logs and request metrics.</Card.Description>
				</Card.Header>
			</Card.Root>
		</a>
	</div>
</div>
