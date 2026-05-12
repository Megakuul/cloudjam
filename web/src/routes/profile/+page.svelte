<script lang="ts">
	import { Button, Dialog, TextField, Toggle, Tooltip } from 'svelte-ux';
	import logo from '$lib/assets/favicon.svg';
	import { create } from '@bufbuild/protobuf';
	import { Glue, setToken, Submit } from '$lib';
	import { onMount } from 'svelte';
	import { GetRequestSchema, UpdateRequestSchema } from '$lib/sdk/v1/admin/user/user_pb';
	import { UserSchema, type User } from '$lib/sdk/v1/admin/user_pb';
	import Gauge from './Gauge.svelte';
	import { goto } from '$app/navigation';
	import WordSwapper from './WordSwapper.svelte';
	import {
		mdiTrashCan,
		mdiFormTextboxPassword,
		mdiEmailEditOutline,
		mdiCheckCircleOutline,
		mdiCloseCircleOutline,
		mdiPencil,
		mdiLogout
	} from '@mdi/js';

	let user: User | undefined = $state();

	let loading = $state(false);
	let error = $state('');
	let edit = $state(false);

	onMount(async () => {
		Submit(
			async () => {
				user = (await Glue.user.get(create(GetRequestSchema, {}))).user;
				user!.score = 95;
				user!.maxScore = 100;
				user!.streak = BigInt(3);
				user!.maxStreak = BigInt(0);
			},
			(e, l) => ((loading = l), (error = e))
		);
	});
</script>

<svelte:head>
	<title>Profile | CloudJam</title>
	<meta property="og:title" content="Profile | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="favicon.png" />
</svelte:head>

<div class="flex flex-col gap-4 justify-center items-center w-full">
	{#if user}
		<div
			class="my-[5%] flex w-full flex-col items-center gap-4 overflow-hidden rounded-2xl border-[0.05rem] border-neutral/40 p-7 shadow-sm shadow-primary/20 lg:flex-row"
		>
			{#if edit}
				<form
					class="flex flex-col gap-1 items-start w-full h-full"
					onsubmit={() =>
						Submit(
							async () => {
								await Glue.user.update(create(UpdateRequestSchema, { mod: user }));
							},
							(e, l) => ((error = e), (loading = l))
						)}
				>
					<div class="flex flex-col gap-4 items-start mt-2 w-full h-full">
						<h1 class="text-4xl opacity-80">Edit User</h1>
						<TextField
							bind:value={user.username}
							class="w-full"
							label="Username"
							placeholder="Change your username"
							type="text"
							dense
							error={Glue.Validate(UserSchema, user).violation.username}
						/>
						<TextField
							bind:value={user.description}
							class="w-full"
							label="Slogan"
							placeholder="Invent a creative Slogan"
							type="text"
							dense
							error={Glue.Validate(UserSchema, user).violation.description}
						/>
						<div class="flex flex-row gap-2 items-center mt-auto">
							<Button on:click={() => (edit = false)} icon={mdiCloseCircleOutline}>Close</Button>
							<Tooltip
								title={Glue.Validate(
									UpdateRequestSchema,
									create(UpdateRequestSchema, { mod: user })
								).error}
								cursor
							>
								<Button
									type="submit"
									variant="fill"
									disabled={Boolean(
										Glue.Validate(UpdateRequestSchema, create(UpdateRequestSchema, { mod: user }))
											.error
									)}
									{loading}
									icon={mdiCheckCircleOutline}
								>
									Update
								</Button>
							</Tooltip>
						</div>
					</div>
				</form>
				<div class="w-full h-1 rounded-2xl lg:w-1 lg:h-64 bg-neutral/80"></div>
				<div class="flex flex-col gap-8 justify-center items-start w-full lg:flex-row">
					<Button icon={mdiEmailEditOutline} on:click={() => console.log('TODO')}
						>Reset Email</Button
					>
					<Button icon={mdiFormTextboxPassword} on:click={() => console.log('TODO')}
						>Change Password</Button
					>
					<Toggle let:on={open} let:toggle let:toggleOff>
						<Button icon={mdiTrashCan} on:click={toggle} color="danger">Delete</Button>
						<Dialog {open} on:close={toggleOff}>
							<div slot="title">Are you sure?</div>
							<div class="py-3 px-6">
								This will permanently delete your account and can not be undone!
							</div>
							<div slot="actions">
								<Button
									on:click={() => {
										console.log('TODO');
									}}
									variant="fill"
									color="danger"
								>
									Yes, delete account
								</Button>
								<Button>Cancel</Button>
							</div>
						</Dialog>
					</Toggle>
				</div>
			{:else}
				<div class="flex flex-col gap-1 items-start w-full">
					<img alt="icon" src={logo} class="h-32" />
					<h1 class="text-4xl opacity-80">{user.username}</h1>
					<p class="mt-auto text-neutral/80">
						Proud CloudJamer since {new Date(Number(user.createdAt) * 1000).toLocaleDateString()}
					</p>
					<div class="flex flex-row gap-2 items-center mt-2">
						<Button on:click={() => (edit = true)} icon={mdiPencil}>Edit</Button>
						<Button
							color="danger"
							on:click={() => {
								setToken('');
								goto('/login');
							}}
							icon={mdiLogout}
							>Logout
						</Button>
					</div>
				</div>
				<div class="w-full h-1 rounded-2xl lg:w-1 lg:h-64 bg-neutral/80"></div>
				<div class="flex flex-col gap-8 justify-end items-center w-full lg:flex-row">
					<!-- TODO add chart -->
					<Gauge
						title="Score"
						scale={30}
						center={user.score}
						outer={user.maxScore}
						inner={user.score}
					/>
					<Gauge
						title="Streak"
						scale={1}
						center={user.streak}
						outer={user.maxStreak}
						inner={user.streak}
					/>
				</div>
			{/if}
		</div>

		<WordSwapper description={user.description} />
	{/if}
	{#if error}
		<div class="p-2 m-2 w-full rounded-sm border-[0.05rem] border-red-600/80 bg-red-600/10">
			{error}
		</div>
	{/if}
</div>
