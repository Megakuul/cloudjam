<script lang="ts">
	import { toSvg } from 'jdenticon';
	import { create } from '@bufbuild/protobuf';
	import { Glue, setToken, Submit, type SubmitState } from '$lib';
	import { onMount } from 'svelte';
	import { timestampFromDate } from '@bufbuild/protobuf/wkt';
	import { GetRequestSchema, ResetPasswordRequestSchema, UpdateRequestSchema } from '$lib/sdk/v1/admin/user/user_pb';
	import { UserSchema, type User } from '$lib/sdk/v1/admin/user_pb';
	import Gauge from './Gauge.svelte';
	import { goto } from '$app/navigation';
	import WordSwapper from './WordSwapper.svelte';
	import Button from '$lib/components/shad/button/button.svelte';
	import { OctagonAlert, CircleCheck, CircleX, Loader, LogOut, Pencil, ShieldCheck } from '@lucide/svelte';
	import Input from '$lib/components/shad/input/input.svelte';
	import Badge from '$lib/components/shad/badge/badge.svelte';
	import * as Dialog from '$lib/components/shad/dialog';
	import * as Alert from '$lib/components/shad/alert';

	let user: User | undefined = $state();

	let userState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let updateState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let resetState: SubmitState = $state({ error: '', loading: false, forbidden: false });
	let edit = $state(false);

	onMount(async () => {
		Submit(async () => {
			user = (await Glue.user.get(create(GetRequestSchema, {}))).user;
		}, userState);
	});
</script>

<svelte:head>
	<title>{user?.username ?? 'Profile'} | CloudJam</title>
	<meta property="og:title" content="Profile | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col items-center justify-center gap-4">
	{#if user}
		<div
			class="border-neutral/40 shadow-primary/20 my-[5%] flex w-full flex-col items-center gap-4 overflow-hidden rounded-2xl border-[0.05rem] p-7 shadow-sm lg:flex-row"
		>
			{#if edit}
				<form
					class="flex h-full w-full flex-col items-start gap-1"
					onsubmit={() =>
						Submit(async () => {
							await Glue.user.update(create(UpdateRequestSchema, { mod: user }));
						}, updateState)}
				>
					<div class="mt-2 flex h-full w-full flex-col items-start gap-4">
						<h1 class="text-4xl opacity-80">Edit User</h1>
						<div class="flex flex-col gap-1">
							<label for="change-username" class="text-sm">Description</label>
							<Input id="change-username" bind:value={user.username} placeholder="Change your username" />
							<p class="text-destructive text-xs">{Glue.Validate(UserSchema, user).violation.username}</p>
						</div>
						<div class="flex flex-col gap-1">
							<label for="change-slogan" class="text-sm">Description</label>
							<Input id="change-slogan" bind:value={user.description} placeholder="Invent a creative Slogan" />
							<p class="text-destructive text-xs">{Glue.Validate(UserSchema, user).violation.description}</p>
						</div>
						<div class="mt-auto flex flex-row items-center gap-2">
							<Button onclick={() => (edit = false)}>
								<CircleX />
								Close
							</Button>
							<Button
								type="submit"
								variant="default"
								disabled={Boolean(Glue.Validate(UpdateRequestSchema, create(UpdateRequestSchema, { mod: user })).error)}
							>
								{#if updateState.loading}
									<Loader />
								{:else}
									<CircleCheck />
								{/if}
								Update
							</Button>
						</div>
						<Alert.Root>
							<OctagonAlert />
							<Alert.Title>Update failed</Alert.Title>
							<Alert.Description class="whitespace-pre-line">{updateState.error}</Alert.Description>
						</Alert.Root>
					</div>
				</form>
				<div class="bg-neutral/80 h-1 w-full rounded-2xl lg:h-64 lg:w-1"></div>
				<div class="flex w-full flex-col items-start justify-center gap-8 lg:flex-row">
					<Dialog.Root>
						<Dialog.Trigger>
							<Button variant="ghost">Change Password</Button>
						</Dialog.Trigger>
						<Dialog.Content>
							<Dialog.Header>
								<Dialog.Title>Reset password?</Dialog.Title>
								<Dialog.Description>This will take you back to the registration process.</Dialog.Description>
							</Dialog.Header>
							<Button
								onclick={() => {
									Submit(async () => {
										const resp = await Glue.user.resetPassword(
											create(ResetPasswordRequestSchema, {
												email: user?.email,
												expires: timestampFromDate(new Date(Date.now() + 10 * 60 * 1000)) // expires in 10 minutes
											})
										);
										goto(`/register?email=${user!.email}&username=${user!.username}&code=${resp.code}`);
									}, resetState);
								}}
								variant="default"
								color="danger"
							>
								{#if resetState.loading}
									<Loader />
								{/if}
								Yes, proceed
							</Button>
							<Alert.Root>
								<OctagonAlert />
								<Alert.Title>Failed to reset</Alert.Title>
								<Alert.Description class="whitespace-pre-line">{resetState.error}</Alert.Description>
							</Alert.Root>
						</Dialog.Content>
					</Dialog.Root>
				</div>
			{:else}
				<div class="flex w-full flex-col items-start gap-1">
					<img
						alt="user profile"
						src={`data:image/svg+xml;base64,${btoa(toSvg(user.pubId, 140))}`}
						height="8rem"
						class="bg-primary/5 rounded-lg"
					/>
					<h1 class="text-4xl opacity-80">{user.username}</h1>
					<p class="text-neutral/80 mt-auto">
						Proud CloudJamer since {new Date(Number(user.createdAt) * 1000).toLocaleDateString()}
					</p>
					<Badge variant="default">{user.organization}</Badge>
					<div class="mt-2 flex flex-row items-center gap-2">
						<Button onclick={() => (edit = true)}>
							<Pencil />
							Edit
						</Button>
						<Button href="/admin" variant="secondary">
							<ShieldCheck />
							Admin
						</Button>
						<Button
							variant="destructive"
							onclick={() => {
								setToken('');
								goto('/login');
							}}
						>
							<LogOut />
							Logout
						</Button>
					</div>
				</div>
				<div class="bg-neutral/80 h-1 w-full rounded-2xl lg:h-64 lg:w-1"></div>
				<div class="flex w-full flex-col items-center justify-end gap-8 lg:flex-row">
					<!-- TODO add chart -->
					<Gauge title="Score" scale={30} center={user.score} outer={user.maxScore} inner={user.score} />
					<Gauge title="Streak" scale={1} center={user.streak} outer={user.maxStreak} inner={user.streak} />
				</div>
			{/if}
		</div>

		<WordSwapper description={user.description} />
	{/if}
	{#if userState.error}
		<Alert.Root>
			<OctagonAlert />
			<Alert.Title>Registration failed</Alert.Title>
			<Alert.Description class="whitespace-pre-line">{userState.error}</Alert.Description>
		</Alert.Root>
	{/if}
</div>
