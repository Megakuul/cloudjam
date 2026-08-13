import {
	ActivityIcon,
	ChartNoAxesCombinedIcon,
	CloudIcon,
	FlagIcon,
	GamepadIcon,
	KeyRoundIcon,
	PlayIcon,
	PodiumIcon,
	ScreenShareIcon,
	ShieldCheckIcon,
	TrophyIcon,
	UsersIcon,
	WandSparklesIcon,
	ZapIcon
} from '@lucide/svelte';
import type { Component } from 'svelte';

export type sidebarSection = {
	icon: Component;
	title: string;
	items: sidebarItem[];
	enabled?: boolean;
};

export type sidebarItem = sidebarGroup | sidebarOption;

export type sidebarGroup = {
	icon: Component;
	title: string;
	collapsible: boolean;
	contents: sidebarOption[];
	enabled: boolean;
};

export type sidebarOption = {
	icon: Component;
	title: string;
	link: string;
	enabled: boolean;
};

export const jamOptions: sidebarSection = {
	enabled: true,
	icon: ZapIcon,
	title: 'Jam',
	items: [
		{
			icon: PlayIcon,
			title: 'Play',
			link: '/play/',
			enabled: true
		}
	]
};

export const hostOptions: sidebarSection = {
	enabled: true,
	icon: ScreenShareIcon,
	title: 'Host',
	items: [
		{
			icon: GamepadIcon,
			title: 'Games',
			link: '/host/games/',
			enabled: true
		},
		{
			icon: WandSparklesIcon,
			title: 'Design',
			link: '/host/design/',
			enabled: true
		}
	]
};

export const providerOptions: sidebarSection = {
	enabled: true,
	icon: CloudIcon,
	title: 'Provider',
	items: [
		{
			icon: KeyRoundIcon,
			title: 'Providers',
			link: '/provider/',
			enabled: true
		}
	]
};

export const adminOptions: sidebarSection = {
	enabled: true,
	icon: ShieldCheckIcon,
	title: 'Administration',
	items: [
		{
			icon: UsersIcon,
			title: 'Users',
			link: '/admin/users/',
			enabled: true
		},
		{
			icon: ShieldCheckIcon,
			title: 'Roles',
			link: '/admin/roles/',
			enabled: true
		},
		{
			icon: ActivityIcon,
			title: 'System',
			link: '/admin/system/',
			enabled: true
		}
	]
};

export const hallOfFameOptions: sidebarSection = {
	enabled: true,
	icon: TrophyIcon,
	title: 'Hall of Fame',
	items: [
		{
			icon: PodiumIcon,
			title: 'Leaderboard',
			link: '/leaderboard',
			enabled: true
		},
		{
			icon: FlagIcon,
			title: 'Tournament',
			link: '/tournament',
			enabled: true
		},
		{
			icon: ChartNoAxesCombinedIcon,
			title: 'Statistics',
			link: '/statistics',
			enabled: true
		}
	]
};
