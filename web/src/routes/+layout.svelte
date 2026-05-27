<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { getSettings, Icon, settings, ThemeSwitch } from 'svelte-ux';
	import { Avatar, Button, Menu, MenuItem, ThemeSelect, Toggle } from 'svelte-ux';
	import { afterNavigate, goto } from '$app/navigation';
	import { getPubId, Glue, setToken } from '$lib';
	import {
		mdiAccount,
		mdiAccountBox,
		mdiAccountOutline,
		mdiLogout,
		mdiShieldCrownOutline
	} from '@mdi/js';
	import { onMount } from 'svelte';
	import { toSvg } from 'jdenticon';

	let { children } = $props();

	settings({
		components: {
			Button: {
				classes: 'cursor-pointer'
			}
		}
	});

	let pubId = $state('');

	onMount(() => {
		pubId = getPubId();
	});

	afterNavigate(() => {
		pubId = getPubId();
	});

	const { currentTheme } = getSettings();
	let locked = $state(true); // avoid theme flickering at page load
	$effect(() => {
		if ($currentTheme.resolvedTheme) locked = false;
	});
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

{#if !locked}
	<div
		class="flex flex-row gap-2 items-center p-2 m-2 rounded-2xl shadow-sm bg-primary/20 shadow-primary/30"
	>
		<div class="ml-auto">
			<ThemeSelect />
		</div>
		<Toggle let:on={open} let:toggle let:toggleOff>
			<Button
				on:click={toggle}
				variant="none"
				color="primary"
				class="cursor-pointer hover:scale-95"
			>
				<Avatar on:click={toggle} class="bg-surface-100/80 text-primary-content">
					{#if pubId}
						<Icon svg={toSvg(pubId, 20)} width="2rem" height="2rem" />
					{:else}
						<Icon path={mdiAccount} width="2rem" height="2rem" />
					{/if}
				</Avatar>
				<Menu {open} on:close={toggleOff} class="w-32">
					<MenuItem
						icon={mdiAccountOutline}
						class="cursor-pointer"
						on:click={() => goto('/profile')}
					>
						Profile
					</MenuItem>
					<MenuItem
						icon={mdiShieldCrownOutline}
						class="cursor-pointer"
						on:click={() => goto('/admin')}
					>
						Admin
					</MenuItem>
					<MenuItem
						icon={mdiLogout}
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
{/if}
