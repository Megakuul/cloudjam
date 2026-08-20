<script lang="ts">
	import logo from '$lib/assets/favicon.svg';
	import { create } from '@bufbuild/protobuf';
	import { LoginRequestSchema } from '$lib/sdk/v1/auth/auth_pb';
	import { Glue, setToken, Submit, type SubmitState } from '$lib';
	import { goto } from '$app/navigation';
	import { Loader, OctagonAlert } from '@lucide/svelte';
	import Input from '$lib/components/shad/input/input.svelte';
	import Button from '$lib/components/shad/button/button.svelte';
	import * as Alert from '$lib/components/shad/alert';

	let loginState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let loginRequest = $state(create(LoginRequestSchema, {}));
</script>

<svelte:head>
	<title>Login | CloudJam</title>
	<meta property="og:title" content="Login | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full items-center justify-center">
	<form
		class="border-neutral/40 shadow-primary/20 mt-[10%] flex w-96 flex-col items-center gap-4 rounded-2xl border-[0.05rem] p-7 shadow-sm"
		onsubmit={() =>
			Submit(async () => {
				setToken((await Glue.auth.login(loginRequest)).token);
				goto('/profile');
			}, loginState)}
	>
		<img alt="icon" src={logo} class="h-32" />
		<h1 class="text-4xl opacity-80">CloudJam Login</h1>
		<Input bind:value={loginRequest.email} placeholder="Please enter your Email" type="email" />
		<Input bind:value={loginRequest.password} placeholder="Please enter your Password" type="password" />
		<Button class="w-full cursor-pointer" type="submit" variant="default">
			{#if loginState.loading}
				<Loader />
			{/if}
			Login
		</Button>
		{#if loginState.error}
			<Alert.Root>
				<OctagonAlert />
				<Alert.Title>Authentication failed</Alert.Title>
				<Alert.Description class="whitespace-pre-line">{loginState.error}</Alert.Description>
			</Alert.Root>
		{/if}
	</form>
</div>
