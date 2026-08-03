import {
	ChartNoAxesCombinedIcon,
	FlagIcon,
	PlayIcon,
	PodiumIcon,
	ScreenShareIcon,
	TrophyIcon,
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
			link: '/play',
			enabled: true
		},
		{
			icon: ScreenShareIcon,
			title: 'Host',
			link: '/host',
			enabled: true
		},
		{
			icon: WandSparklesIcon,
			title: 'Design',
			link: '/design',
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
