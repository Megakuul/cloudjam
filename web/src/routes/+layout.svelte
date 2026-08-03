<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { afterNavigate, goto } from '$app/navigation';
	import { getEmail, getPubId, Glue, setToken } from '$lib';
	import { onMount } from 'svelte';
	import { toSvg } from 'jdenticon';
	import { mode, ModeWatcher, toggleMode } from 'mode-watcher';
	import {
		SunIcon,
		MoonIcon,
		ChevronDown,
		ChevronUp,
		LogOut,
		User,
		Play,
		ScreenShare,
		WandSparkles,
		Podium,
		Flag,
		ChartNoAxesCombined
	} from '@lucide/svelte';
	import Button from '$lib/components/shad/button/button.svelte';
	import * as DropdownMenu from '$lib/components/shad/dropdown-menu';
	import Badge from '$lib/components/shad/badge/badge.svelte';

	let { children } = $props();

	let pubId = $state('');
	let email = $state('');

	onMount(() => {
		pubId = getPubId();
		email = getEmail();
	});

	afterNavigate(() => {
		pubId = getPubId();
		email = getEmail();
	});

	let jamDropdown = $state(false);
	let hofDropdown = $state(false);
	let userDropdown = $state(false);
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

<ModeWatcher />

<div class="flex flex-row gap-1 justify-end items-center p-2 w-full">
	<DropdownMenu.Root bind:open={jamDropdown}>
		<DropdownMenu.Trigger>
			<Button variant="ghost" class="h-12 p-4 font-bold {jamDropdown ? 'bg-accent text-accent-foreground' : ''}">
				⚡️ Jam
				<ChevronDown class="transition-transform duration-200 {jamDropdown ? 'rotate-180' : ''}" />
			</Button>
		</DropdownMenu.Trigger>
		<DropdownMenu.Content align="end">
			<DropdownMenu.Group>
				<DropdownMenu.Label>Jam</DropdownMenu.Label>
				<DropdownMenu.Separator />
				<DropdownMenu.Item>
					<Play />
					<a href="/play">Play</a>
				</DropdownMenu.Item>
				<DropdownMenu.Item>
					<ScreenShare />
					<a href="/host">Host</a>
				</DropdownMenu.Item>
				<DropdownMenu.Item>
					<WandSparkles />
					<a href="/design">Design</a>
				</DropdownMenu.Item>
			</DropdownMenu.Group>
		</DropdownMenu.Content>
	</DropdownMenu.Root>

	<DropdownMenu.Root bind:open={hofDropdown}>
		<DropdownMenu.Trigger>
			<Button variant="ghost" class="h-12 p-4 font-bold {hofDropdown ? 'bg-accent text-accent-foreground' : ''}">
				🏆 HoF
				<ChevronDown class="transition-transform duration-200 {hofDropdown ? 'rotate-180' : ''}" />
			</Button>
		</DropdownMenu.Trigger>
		<DropdownMenu.Content align="end">
			<DropdownMenu.Group>
				<DropdownMenu.Label>Hall of Fame</DropdownMenu.Label>
				<DropdownMenu.Separator />
				<DropdownMenu.Item>
					<Podium />
					<a href="/leaderboard">Leaderboard</a>
				</DropdownMenu.Item>
				<DropdownMenu.Item>
					<Flag />
					<a href="/tournament">Tournament</a>
				</DropdownMenu.Item>
				<DropdownMenu.Item>
					<ChartNoAxesCombined />
					<a href="/statistics">Statistics</a>
				</DropdownMenu.Item>
			</DropdownMenu.Group>
		</DropdownMenu.Content>
	</DropdownMenu.Root>

	<DropdownMenu.Root bind:open={userDropdown}>
		<DropdownMenu.Trigger>
			<Button variant="ghost" class="h-12 p-4 {userDropdown ? 'bg-accent text-accent-foreground' : ''}">
				<img
					alt="user profile"
					src={`data:image/svg+xml;base64,${btoa(toSvg(pubId, 40))}`}
					class="rounded-lg bg-primary/5"
				/>
				<ChevronDown class="transition-transform duration-200 {userDropdown ? 'rotate-180' : ''}" />
			</Button>
		</DropdownMenu.Trigger>
		<DropdownMenu.Content align="end">
			<DropdownMenu.Group>
				<DropdownMenu.Label>{email}</DropdownMenu.Label>
				<DropdownMenu.Separator />
				<DropdownMenu.Item>
					<User />
					<a href="/profile">Profile</a>
				</DropdownMenu.Item>
				<DropdownMenu.Item
					onclick={(e) => {
						e.stopPropagation();
						e.preventDefault();
						toggleMode();
					}}
				>
					<SunIcon class="h-[1.2rem] w-[1.2rem] scale-100 rotate-0 transition-all! dark:scale-0 dark:-rotate-90" />
					<MoonIcon
						class="absolute h-[1.2rem] w-[1.2rem] scale-0 rotate-90 transition-all! dark:scale-100 dark:rotate-0"
					/>
					Theme
				</DropdownMenu.Item>
				<DropdownMenu.Item
					variant="destructive"
					onclick={() => {
						setToken('');
						goto('/login');
					}}
				>
					<LogOut />
					Logout
				</DropdownMenu.Item>
			</DropdownMenu.Group>
		</DropdownMenu.Content>
	</DropdownMenu.Root>
</div>

<div class="p-4 w-full">
	{@render children()}
</div>
