<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { settings } from 'svelte-ux';
	import { Avatar, Button, Menu, MenuItem, ThemeSelect, Toggle } from 'svelte-ux';
	import { goto } from '$app/navigation';
	import { setToken } from '$lib';

	let { children } = $props();

	let theme = $state('white');
	$effect(() => {
		document.documentElement.setAttribute('theme', theme);
	});

	settings({
		components: {
			Button: {
				classes: 'cursor-pointer',
				variant: 'outline'
			}
		}
	});
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

<div
	class="flex flex-row gap-2 items-center p-2 m-2 rounded-2xl shadow-sm bg-primary/20 shadow-primary/30"
>
	<div class="ml-auto">
		<ThemeSelect />
	</div>
	<Toggle let:on={open} let:toggle let:toggleOff>
		<Button on:click={toggle} variant="none" color="primary" class="cursor-pointer hover:scale-95">
			<Avatar on:click={toggle} class="bg-primary/60 text-primary-content">A</Avatar>
			<Menu {open} on:close={toggleOff}>
				<MenuItem class="cursor-pointer" on:click={() => goto('/profile')}>Profile</MenuItem>
				<MenuItem
					class="cursor-pointer"
					on:click={() => {
						setToken('');
						goto('/login');
					}}
				>
					Logout
				</MenuItem>
			</Menu>
		</Button>
	</Toggle>
</div>

<div class="p-4 w-full">
	{@render children()}
</div>
