<script lang="ts">
	import logo from '$lib/assets/favicon.svg';
	import { create } from '@bufbuild/protobuf';
	import { RegisterRequestSchema } from '$lib/sdk/v1/auth/auth_pb';
	import { Glue, Submit, type SubmitState } from '$lib';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import Input from '$lib/components/shad/input/input.svelte';
	import Button from '$lib/components/shad/button/button.svelte';
	import * as Alert from '$lib/components/shad/alert';
	import { Loader, OctagonAlert } from '@lucide/svelte';

	let registerRequest = $state(create(RegisterRequestSchema, {}));

	let registerState: SubmitState = $state({ error: '', loading: false, forbidden: false });

	onMount(() => {
		registerRequest.email = page.url.searchParams.get('email') ?? '';
		registerRequest.username = page.url.searchParams.get('username') ?? '';
		registerRequest.code = page.url.searchParams.get('code') ?? '';
		history.replaceState(null, '', '/register');
	});
</script>

<svelte:head>
	<title>Register | CloudJam</title>
	<meta property="og:title" content="RegisteRegister | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full items-center justify-center">
	<form
		class="border-neutral/40 shadow-primary/20 mt-[10%] flex w-96 flex-col items-center gap-4 rounded-2xl border-[0.05rem] p-7 shadow-sm"
		onsubmit={() =>
			Submit(async () => {
				await Glue.auth.register(registerRequest);
				goto('/login');
			}, registerState)}
	>
		<img alt="icon" src={logo} class="h-32" />
		<h1 class="text-4xl opacity-80">Registration</h1>
		<Input bind:value={registerRequest.email} class="w-full" placeholder="Please enter your Email" type="email" />
		<Input
			bind:value={registerRequest.code}
			class="w-full"
			placeholder="Please enter your registration code"
			type="text"
		/>
		<hr class="border-neutral/40 w-full border" />
		<Input bind:value={registerRequest.username} class="w-full" placeholder="Create a creative username" type="text" />
		<Input
			bind:value={registerRequest.password}
			class="w-full"
			placeholder="Create a supersecret password"
			type="password"
		/>
		<Input
			bind:value={registerRequest.confirmPassword}
			class="w-full"
			placeholder="Confirm your supersecret password"
			type="password"
		/>
		<Button
			class="w-full cursor-pointer"
			type="submit"
			disabled={Boolean(Glue.Validate(RegisterRequestSchema, registerRequest).error)}
			variant="default"
		>
			Register
			{#if registerState.loading}
				<Loader />
			{/if}
		</Button>
		{#if registerState.error}
			<Alert.Root>
				<OctagonAlert />
				<Alert.Title>Registration failed</Alert.Title>
				<Alert.Description class="whitespace-pre-line">{registerState.error}</Alert.Description>
			</Alert.Root>
		{/if}
	</form>
</div>
