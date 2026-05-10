<script lang="ts">
	import { Button, Gooey, TextField } from 'svelte-ux';
	import logo from '$lib/assets/favicon.svg';
	import { create } from '@bufbuild/protobuf';
	import { LoginRequestSchema } from '$lib/sdk/v1/auth/auth_pb';
	import { Glue, setToken, Submit } from '$lib';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { GetRequestSchema, UpdateRequestSchema } from '$lib/sdk/v1/admin/user/user_pb';
	import type { User } from '$lib/sdk/v1/admin/user_pb';
	import { blur } from 'svelte/transition';
	import { circIn, circOut } from 'svelte/easing';

	let user: User | undefined = $state();
	let loading = $state(false);
	let error = $state('');

	onMount(async () => {
		Submit(
			async () => {
				user = (await Glue.user.get(create(GetRequestSchema, {}))).user;
			},
			(e, l) => ((loading = l), (error = e))
		);
	});

	let currentWord = $state('');

	onMount(() => {
		const interval = setInterval(() => {
			if (user) {
				const words = user.description.split(' ');
				if (words.length < 1) {
					currentWord = '';
					return;
				}
				// designed this weird to be resilient if the desc is changed.
				const currentIdx = words.indexOf(currentWord);
				if (currentIdx === -1 || currentIdx >= words.length) currentWord = words[0];
				else currentWord = words[currentIdx + 1];
			}
		}, 1000);
		return () => clearInterval(interval);
	});
</script>

<svelte:head>
	<title>Profile | CloudJam</title>
	<meta property="og:title" content="Profile | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="favicon.png" />
</svelte:head>

<div class="flex justify-center items-center w-full">
	{#if user}
		<Gooey blur={4} alphaPixel={255} alphaShift={-144}>
			<div class="grid place-items-center text-8xl font-bold text-center w-[500px] grid-stack">
				{#key currentWord}
					<span
						in:blur={{ amount: '10px', duration: 1000, easing: circOut }}
						out:blur={{ amount: '100px', duration: 1000, easing: circIn }}
					>
						{currentWord}
					</span>
				{/key}
			</div>
		</Gooey>
		<div
			class="mt-[10%] flex w-96 flex-col items-center gap-4 rounded-2xl border-[0.05rem] border-neutral/40 p-7 shadow-sm shadow-primary/20"
		>
			<img alt="icon" src={logo} class="h-32" />
			<h1 class="text-4xl opacity-80">{user.username}</h1>
			<p>{user.email}</p>
			<p>{user.description}</p>
		</div>
	{/if}
	{#if error}
		<div class="p-2 w-full rounded-sm border-[0.05rem] border-red-600/80 bg-red-600/10">
			{error}
		</div>
	{/if}
</div>
