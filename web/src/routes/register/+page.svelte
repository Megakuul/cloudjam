<script lang="ts">
	import logo from '$lib/assets/favicon.svg';
	import { create } from '@bufbuild/protobuf';
	import { RegisterRequestSchema } from '$lib/sdk/v1/auth/auth_pb';
	import { Glue, Submit } from '$lib';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import Input from '$lib/components/ui/input/input.svelte';
	import Button from '$lib/components/ui/button/button.svelte';
	import * as Alert from '$lib/components/ui/alert';
	import { Loader, OctagonAlert } from '@lucide/svelte';

	let request = $state(create(RegisterRequestSchema, {}));
	let loading = $state(false);
	let error = $state('');

	onMount(() => {
		request.email = page.url.searchParams.get('email') ?? '';
		request.username = page.url.searchParams.get('username') ?? '';
		request.code = page.url.searchParams.get('code') ?? '';
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
		class="mt-[10%] flex w-96 flex-col items-center gap-4 rounded-2xl border-[0.05rem] border-neutral/40 p-7 shadow-sm shadow-primary/20"
		onsubmit={() =>
			Submit(
				async () => {
					await Glue.auth.register(request);
					goto('/login');
				},
				(e, l) => ((error = e), (loading = l))
			)}
	>
		<img alt="icon" src={logo} class="h-32" />
		<h1 class="text-4xl opacity-80">Registration</h1>
		<Input
			bind:value={request.email}
			class="w-full"
			placeholder="Please enter your Email"
			type="email"
			error={Glue.Validate(RegisterRequestSchema, request).violation.email}
		/>
		<Input
			bind:value={request.code}
			class="w-full"
			placeholder="Please enter your registration code"
			type="text"
			error={Glue.Validate(RegisterRequestSchema, request).violation.code}
		/>
		<hr class="w-full border border-neutral/40" />
		<Input
			bind:value={request.username}
			class="w-full"
			placeholder="Create a creative username"
			type="text"
			error={Glue.Validate(RegisterRequestSchema, request).violation.username}
		/>
		<Input
			bind:value={request.password}
			class="w-full"
			placeholder="Create a supersecret password"
			type="password"
			error={Glue.Validate(RegisterRequestSchema, request).violation.password}
		/>
		<Input
			bind:value={request.confirmPassword}
			class="w-full"
			placeholder="Confirm your supersecret password"
			type="password"
			error={Glue.Validate(RegisterRequestSchema, request).error}
		/>
		<Button
			class="w-full cursor-pointer"
			type="submit"
			disabled={Boolean(Glue.Validate(RegisterRequestSchema, request).error)}
			variant="default"
		>
			Register
			{#if loading}
				<Loader />
			{/if}
		</Button>
		{#if error}
			<Alert.Root>
				<OctagonAlert />
				<Alert.Title>Registration failed</Alert.Title>
				<Alert.Description class="whitespace-pre-line">{error}</Alert.Description>
			</Alert.Root>
		{/if}
	</form>
</div>
