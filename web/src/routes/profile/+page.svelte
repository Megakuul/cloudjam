<script lang="ts">
	import { toSvg } from 'jdenticon';
	import { create } from '@bufbuild/protobuf';
	import { Glue, setToken, Submit } from '$lib';
	import { onMount } from 'svelte';
	import { timestampFromDate } from '@bufbuild/protobuf/wkt';
	import { GetRequestSchema, ResetPasswordRequestSchema, UpdateRequestSchema } from '$lib/sdk/v1/admin/user/user_pb';
	import { UserSchema, type User } from '$lib/sdk/v1/admin/user_pb';
	import Gauge from './Gauge.svelte';
	import { goto } from '$app/navigation';
	import WordSwapper from './WordSwapper.svelte';
	import Button from '$lib/components/ui/button/button.svelte';
	import { CircleCheck, CircleX, Loader, LogOut, Pencil, ShieldCheck } from '@lucide/svelte';
	import Input from '$lib/components/ui/input/input.svelte';
	import Badge from '$lib/components/ui/badge/badge.svelte';
	import * as Dialog from '$lib/components/ui/dialog';

	let user: User | undefined = $state();

	let loading = $state(false);
	let error = $state('');
	let edit = $state(false);

	onMount(async () => {
		Submit(
			async () => {
				user = (await Glue.user.get(create(GetRequestSchema, {}))).user;
			},
			(e, l) => ((loading = l), (error = e))
		);
	});
</script>

<svelte:head>
	<title>Profile | CloudJam</title>
	<meta property="og:title" content="Profile | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col items-center justify-center gap-4">
	{#if user}
		<div
			class="my-[5%] flex w-full flex-col items-center gap-4 overflow-hidden rounded-2xl border-[0.05rem] border-neutral/40 p-7 shadow-sm shadow-primary/20 lg:flex-row"
		>
			{#if edit}
				<form
					class="flex h-full w-full flex-col items-start gap-1"
					onsubmit={() =>
						Submit(
							async () => {
								await Glue.user.update(create(UpdateRequestSchema, { mod: user }));
							},
							(e, l) => ((error = e), (loading = l))
						)}
				>
					<div class="mt-2 flex h-full w-full flex-col items-start gap-4">
						<h1 class="text-4xl opacity-80">Edit User</h1>
						<Input
							bind:value={user.username}
							class="w-full"
							placeholder="Change your username"
							type="text"
							error={Glue.Validate(UserSchema, user).violation.username}
						/>
						<Input
							bind:value={user.description}
							class="w-full"
							placeholder="Invent a creative Slogan"
							type="text"
							error={Glue.Validate(UserSchema, user).violation.description}
						/>
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
								{#if loading}
									<Loader />
								{:else}
									<CircleCheck />
								{/if}
								Update
							</Button>
						</div>
					</div>
				</form>
				<div class="h-1 w-full rounded-2xl bg-neutral/80 lg:h-64 lg:w-1"></div>
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
									Submit(
										async () => {
											const resp = await Glue.user.resetPassword(
												create(ResetPasswordRequestSchema, {
													email: user?.email,
													expires: timestampFromDate(new Date(Date.now() + 10 * 60 * 1000)) // expires in 10 minutes
												})
											);
											goto(`/register?email=${user!.email}&username=${user!.username}&code=${resp.code}`);
										},
										(e, l) => ((error = e), (loading = l))
									);
								}}
								variant="default"
								color="danger"
							>
								{#if loading}
									<Loader />
								{/if}
								Yes, proceed
							</Button>
						</Dialog.Content>
					</Dialog.Root>
				</div>
			{:else}
				<div class="flex w-full flex-col items-start gap-1">
					<img
						alt="user profile"
						src={`data:image/svg+xml;base64,${btoa(toSvg(user.pubId, 140))}`}
						height="8rem"
						class="rounded-lg bg-primary/5"
					/>
					<h1 class="text-4xl opacity-80">{user.username}</h1>
					<p class="mt-auto text-neutral/80">
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
				<div class="h-1 w-full rounded-2xl bg-neutral/80 lg:h-64 lg:w-1"></div>
				<div class="flex w-full flex-col items-center justify-end gap-8 lg:flex-row">
					<!-- TODO add chart -->
					<Gauge title="Score" scale={30} center={user.score} outer={user.maxScore} inner={user.score} />
					<Gauge title="Streak" scale={1} center={user.streak} outer={user.maxStreak} inner={user.streak} />
				</div>
			{/if}
		</div>

		<WordSwapper description={user.description} />
	{/if}
	{#if error}
		<div class="m-2 w-full rounded-sm border-[0.05rem] border-red-600/80 bg-red-600/10 p-2">
			{error}
		</div>
	{/if}
</div>
