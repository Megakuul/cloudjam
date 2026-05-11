<script lang="ts">
	import { Button, TextField } from 'svelte-ux';
	import logo from '$lib/assets/favicon.svg';
	import { create } from '@bufbuild/protobuf';
	import { RegisterRequestSchema } from '$lib/sdk/v1/auth/auth_pb';
	import { Glue, Submit } from '$lib';
	import { goto } from '$app/navigation';

	let request = $state(create(RegisterRequestSchema, {}));
	let loading = $state(false);
	let error = $state('');
</script>

<svelte:head>
	<title>Register | CloudJam</title>
	<meta property="og:title" content="RegisteRegister | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="favicon.png" />
</svelte:head>

<div class="flex justify-center items-center w-full">
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
		<TextField
			bind:value={request.email}
			class="w-full"
			label="Email"
			placeholder="Please enter your Email"
			type="email"
			dense
			error={Glue.Validate(RegisterRequestSchema, request).violation.email}
		/>
		<TextField
			bind:value={request.code}
			class="w-full"
			label="Code"
			placeholder="Please enter your registration code"
			type="text"
			dense
			error={Glue.Validate(RegisterRequestSchema, request).violation.code}
		/>
		<hr class="w-full border border-neutral/40" />
		<TextField
			bind:value={request.username}
			class="w-full"
			label="Username"
			placeholder="Create a creative username"
			type="text"
			dense
			error={Glue.Validate(RegisterRequestSchema, request).violation.username}
		/>
		<TextField
			bind:value={request.password}
			class="w-full"
			label="Password"
			placeholder="Create a supersecret password"
			type="password"
			dense
			error={Glue.Validate(RegisterRequestSchema, request).violation.password}
		/>
		<TextField
			bind:value={request.confirmPassword}
			class="w-full"
			label="Password (again)"
			placeholder="Confirm your supersecret password"
			type="password"
			dense
			error={Glue.Validate(RegisterRequestSchema, request).error}
		/>
		<Button
			class="w-full cursor-pointer"
			type="submit"
			{loading}
			disabled={Boolean(Glue.Validate(RegisterRequestSchema, request).error)}
			variant="fill">Register</Button
		>
		{#if error}
			<div class="p-2 w-full rounded-sm border-[0.05rem] border-red-600/80 bg-red-600/10">
				{error}
			</div>
		{/if}
	</form>
</div>
