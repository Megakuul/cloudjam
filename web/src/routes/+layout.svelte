<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { Avatar, Button, Menu, MenuItem, Settings, ThemeSelect, Toggle } from 'svelte-ux';

	let { children } = $props();

	let theme = $state('white');
	$effect(() => {
		document.documentElement.setAttribute('theme', theme);
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
				<MenuItem>Settings</MenuItem>
				<MenuItem>Sign In</MenuItem>
				<MenuItem disabled>Disabled</MenuItem>
			</Menu>
		</Button>
	</Toggle>
</div>

{@render children()}
