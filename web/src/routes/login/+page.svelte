<script lang="ts">
	import logo from '$lib/assets/favicon.svg';
	import { create } from '@bufbuild/protobuf';
	import { LoginRequestSchema } from '$lib/sdk/v1/auth/auth_pb';
	import { Glue, setToken, Submit } from '$lib';
	import { goto } from '$app/navigation';
	import { Loader, OctagonAlert } from '@lucide/svelte';
	import Input from '$lib/components/ui/input/input.svelte';
	import Button from '$lib/components/ui/button/button.svelte';
	import * as Alert from '$lib/components/ui/alert';
	import { fade } from 'svelte/transition';

	let request = $state(create(LoginRequestSchema, {}));
	let loading = $state(false);
	let error = $state('');
</script>

<svelte:head>
	<title>Login | CloudJam</title>
	<meta property="og:title" content="Login | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex justify-center items-center w-full">
	<form
		class="mt-[10%] flex w-96 flex-col items-center gap-4 rounded-2xl border-[0.05rem] border-neutral/40 p-7 shadow-sm shadow-primary/20"
		onsubmit={() =>
			Submit(
				async () => {
					setToken((await Glue.auth.login(request)).token);
					goto('/profile');
				},
				(e, l) => ((error = e), (loading = l))
			)}
	>
		<img alt="icon" src={logo} class="h-32" />
		<h1 class="text-4xl opacity-80">CloudJam Login</h1>
		<Input bind:value={request.email} placeholder="Please enter your Email" type="email" />
		<Input bind:value={request.password} placeholder="Please enter your Password" type="password" />
		<Button class="w-full cursor-pointer" type="submit" variant="default">
			{#if loading}
				<Loader />
			{/if}
			Login
		</Button>
		{#if error}
			<Alert.Root>
				<OctagonAlert />
				<Alert.Title>Authentication failed</Alert.Title>
				<Alert.Description class="whitespace-pre-line">{error}</Alert.Description>
			</Alert.Root>
		{/if}
	</form>
</div>
