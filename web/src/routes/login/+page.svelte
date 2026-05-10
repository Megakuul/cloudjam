<script lang="ts">
	import { Button, TextField } from 'svelte-ux';
	import logo from '$lib/assets/favicon.svg';
	import { create } from '@bufbuild/protobuf';
	import { LoginRequestSchema } from '$lib/sdk/v1/auth/auth_pb';
	import { Glue, setToken, Submit } from '$lib';
	import { goto } from '$app/navigation';

	let request = $state(create(LoginRequestSchema, {}));
	let loading = $state(false);
	let error = $state('');
</script>

<svelte:head>
	<title>Login | CloudJam</title>
	<meta property="og:title" content="Login | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="favicon.png" />
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
		<TextField
			bind:value={request.email}
			class="w-full"
			label="Email"
			placeholder="Please enter your Email"
			type="email"
			error={Glue.Validate(LoginRequestSchema, request).violation.email}
		/>

		<TextField
			bind:value={request.password}
			class="w-full"
			label="Password"
			placeholder="Please enter your Password"
			type="password"
			error={Glue.Validate(LoginRequestSchema, request).violation.password}
		/>
		<Button
			class="w-full cursor-pointer"
			type="submit"
			{loading}
			disabled={Boolean(Glue.Validate(LoginRequestSchema, request).error)}
			variant="fill">Login</Button
		>
		{#if error}
			<div class="p-2 w-full rounded-sm border-[0.05rem] border-red-600/80 bg-red-600/10">
				{error}
			</div>
		{/if}
	</form>
</div>
