<script lang="ts">
	import * as Sidebar from '$lib/components/shad/sidebar';
	import * as Popover from '$lib/components/shad/popover';
	import * as Field from '$lib/components/shad/field';
	import { page } from '$app/state';
	import { useSidebar } from '$lib/components/shad/sidebar';
	import AvatarRenderer from '$lib/components/custom/renderers/AvatarRenderer.svelte';
	import { Skeleton } from '$lib/components/shad/skeleton';
	import {
		ChevronsUpDownIcon,
		ShieldPlusIcon,
		ShieldIcon,
		UserIcon,
		BadgeQuestionMarkIcon,
		CogIcon,
		SunIcon,
		MoonIcon,
		LoaderCircleIcon,
		LogOutIcon
	} from '@lucide/svelte';
	import WrappedTooltip from '$lib/components/custom/WrappedTooltip.svelte';
	import { mode, resetMode, setMode } from 'mode-watcher';
	import { Button, buttonVariants } from '$lib/components/shad/button';
	import { toast } from 'svelte-sonner';

	const sidebar = useSidebar();
	let { loading = $bindable(true) }: { loading: boolean } = $props();

	let open = $state(false);
	let themeChanging = $state(false);

	let user = $derived(
		page.data.session && page.data.session.user
			? page.data.session.user
			: {
					name: 'NA',
					image: null,
					permission: 'STANDARD',
					email: 'na@notfound.com'
				}
	);

	// Theme Stuff
	type ThemeMode = 'light' | 'dark' | 'system';

	async function setTheme(nextTheme: ThemeMode) {
		if (themeChanging || mode.current === nextTheme) return;

		themeChanging = true;
		try {
			switch (nextTheme) {
				case 'light':
				case 'dark':
					setMode(nextTheme);
					break;
				default:
					resetMode();
					break;
			}

			toast.success(`Theme changed!`);
		} catch (e) {
			toast.error('Unable to change theme! Please report this as a bug!');
			console.error(e);
		}
		themeChanging = false;
	}

	async function toggleTheme() {
		await setTheme(mode.current === 'light' ? 'dark' : 'light');
	}
</script>

<Sidebar.MenuItem>
	<Popover.Root bind:open>
		<Popover.Trigger disabled={loading}>
			{#snippet child({ props })}
				<Sidebar.MenuButton
					{...props}
					size="lg"
					class="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
				>
					{#if loading}
						<div
							class="flex aspect-square size-8 items-center justify-center rounded-lg text-sidebar-primary-foreground"
						>
							<Skeleton class="h-8 w-8 rounded-full" />
						</div>
						<div class="grid flex-1 text-start text-sm leading-tight">
							<span class="truncate font-medium">
								<Skeleton class="h-6 w-full" />
							</span>
						</div>
					{:else}
						<div
							class="flex aspect-square size-8 items-center justify-center rounded-lg text-sidebar-primary-foreground"
						>
							<AvatarRenderer image={user.image ?? ''} name={user.name ?? 'Unknown'} width="6" height="6" />
						</div>
						<div class="grid flex-1 text-start text-sm leading-tight">
							<span>{user.name ?? 'Unknown'}</span>
						</div>
					{/if}

					<ChevronsUpDownIcon class="ms-auto" />
				</Sidebar.MenuButton>
			{/snippet}
		</Popover.Trigger>

		<Popover.Content
			align="start"
			class="w-(--bits-dropdown-menu-anchor-width) min-w-48 rounded-lg"
			side={sidebar.isMobile ? 'top' : 'right'}
			sideOffset={15}
		>
			<Popover.Header class="flex flex-col items-center">
				<AvatarRenderer image={user.image ?? ''} name={user.name ?? 'Unknown'} width="w-20" height="h-20" />
				<Popover.Title class="inline-flex items-center text-2xl font-bold">
					{user.name}
					<span class="ml-2">
						{#if user.permission === 'SUPERADMIN'}
							<WrappedTooltip caption="Super Admin">
								<ShieldPlusIcon class="h-6 w-6 fill-yellow-500 stroke-yellow-700" />
							</WrappedTooltip>
						{:else if user.permission === 'ADMIN'}
							<WrappedTooltip caption="Admin">
								<ShieldIcon class="h-6 w-6 fill-primary stroke-secondary-foreground" />
							</WrappedTooltip>
						{:else if user.permission === 'STANDARD'}
							<WrappedTooltip caption="Standard">
								<UserIcon class="h-6 w-6" />
							</WrappedTooltip>
						{:else}
							<WrappedTooltip caption="Unknown - Please Report this!">
								<BadgeQuestionMarkIcon class="h-6 w-6" />
							</WrappedTooltip>
						{/if}
					</span>
				</Popover.Title>
				<Popover.Description>{user.email}</Popover.Description>
			</Popover.Header>

			<Field.Group>
				<Field.Title>
					<CogIcon class="h-4 w-4" />
					Settings
				</Field.Title>

				<Field.Content>
					<Field.Group>
						<Field.Field orientation="horizontal" class="w-full">
							<Field.Label>Theme</Field.Label>
							<Button
								variant="outline"
								class="mr-auto ml-auto"
								onclick={async () => await toggleTheme()}
								disabled={themeChanging}
								aria-busy={themeChanging}
							>
								{#if themeChanging}
									<LoaderCircleIcon class="animate-spin" />
								{:else if mode.current === 'light'}
									<SunIcon class="" />
								{:else}
									<MoonIcon class="" />
								{/if}
							</Button>
						</Field.Field>
					</Field.Group>
				</Field.Content>
			</Field.Group>

			<!--				<SignOut class="mx-auto">-->
			<span class={buttonVariants({ variant: 'destructive' }) + ' mt-4'} slot="submitButton">
				<LogOutIcon class="mr-2" />
				Logout
			</span>
			<!--				</SignOut>-->
		</Popover.Content>
	</Popover.Root>
</Sidebar.MenuItem>
