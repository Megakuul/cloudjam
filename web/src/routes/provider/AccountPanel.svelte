<script lang="ts">
	import { Glue, Submit } from '$lib';
	import * as Alert from '$lib/components/shad/alert';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';
	import { Separator } from '$lib/components/shad/separator';
	import { DeleteRequestSchema, FixRequestSchema, UpdateRequestSchema } from '$lib/sdk/v1/cloud/account/account_pb';
	import { AccountState, type Account } from '$lib/sdk/v1/cloud/account_pb';
	import { create } from '@bufbuild/protobuf';
	import { timestampDate } from '@bufbuild/protobuf/wkt';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';

	let { account, refresh }: { account: Account; refresh: () => void } = $props();

	type section = { error: string; loading: boolean; forbidden: boolean };
	const setter = (s: section) => (e: string, l: boolean, f: boolean) => (
		(s.error = e),
		(s.loading = l),
		(s.forbidden = f)
	);

	// the panel is remounted (keyed) per account, capturing the initial value is intended.
	// svelte-ignore state_referenced_locally
	let description = $state(account.description);
	let confirmDelete = $state(false);
	let force = $state(false);

	let update: section = $state({ error: '', loading: false, forbidden: false });
	let fix: section = $state({ error: '', loading: false, forbidden: false });
	let remove: section = $state({ error: '', loading: false, forbidden: false });
</script>

<Card.Root class="w-full">
	<Card.Header>
		<Card.Title class="text-2xl">{account.name}</Card.Title>
		<Card.Description>{account.description}</Card.Description>
		<div class="flex flex-row flex-wrap gap-1">
			<Badge variant={account.state === AccountState.Corrupted ? 'destructive' : 'secondary'}>
				{AccountState[account.state]}
			</Badge>
			<Badge variant="outline">scope: {account.scope}</Badge>
			{#if account.targetId}
				<Badge variant="outline" class="font-mono">target: {account.targetId}</Badge>
			{/if}
			{#if account.boundUntil}
				<Badge variant="outline">bound until: {timestampDate(account.boundUntil).toLocaleString()}</Badge>
			{/if}
		</div>
	</Card.Header>
	<Card.Content class="flex flex-col gap-6">
		{#if account.error}
			<Alert.Root variant="destructive">
				<AlertCircleIcon />
				<Alert.Title>Provider reported an error</Alert.Title>
				<Alert.Description class="font-mono text-xs break-all">{account.error}</Alert.Description>
			</Alert.Root>
		{/if}

		<div class="flex flex-col gap-2">
			<Card.Title>Description</Card.Title>
			{#if update.forbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to update this account.</p>
			{:else}
				<p class="text-muted-foreground text-sm">Only accounts in the ready state accept metadata updates.</p>
				<form
					class="flex flex-row items-center gap-2"
					onsubmit={() =>
						Submit(async () => {
							await Glue.account.update(create(UpdateRequestSchema, { mod: { ...account, description: description } }));
							refresh();
						}, setter(update))}
				>
					<Input class="max-w-96" bind:value={description} placeholder="Purpose of the account" />
					<Button type="submit" variant="outline" class="cursor-pointer" disabled={update.loading}>Save</Button>
				</form>
				{#if update.error}
					<p class="text-destructive text-xs">{update.error}</p>
				{/if}
			{/if}
		</div>

		<Separator />

		<div class="flex flex-col gap-2">
			<Card.Title>Recovery</Card.Title>
			{#if fix.forbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to fix this account.</p>
			{:else}
				<p class="text-muted-foreground text-sm">
					Forces the account back into the ready state. Only do this after you actually repaired the account on the
					provider, otherwise it is handed out broken.
				</p>
				<Button
					variant="outline"
					class="cursor-pointer self-start"
					disabled={fix.loading || account.state !== AccountState.Corrupted}
					onclick={() =>
						Submit(async () => {
							await Glue.account.fix(create(FixRequestSchema, { providerId: account.providerId, id: account.id }));
							refresh();
						}, setter(fix))}
				>
					Mark as Fixed
				</Button>
				{#if fix.error}
					<p class="text-destructive text-xs">{fix.error}</p>
				{/if}
			{/if}
		</div>

		<Separator />

		<div class="flex flex-col gap-2">
			<Card.Title>Danger Zone</Card.Title>
			{#if remove.forbidden}
				<p class="text-muted-foreground text-sm italic">You are not allowed to delete this account.</p>
			{:else}
				<label class="text-muted-foreground flex flex-row items-center gap-2 text-sm">
					<input type="checkbox" bind:checked={force} class="cursor-pointer" />
					Force: drop the CloudJam metadata immediately without waiting for the provider (the account leaks).
				</label>
				<div class="flex flex-row items-center gap-2">
					{#if confirmDelete}
						<Button
							variant="destructive"
							class="cursor-pointer"
							disabled={remove.loading}
							onclick={() =>
								Submit(async () => {
									await Glue.account.delete(
										create(DeleteRequestSchema, {
											providerId: account.providerId,
											id: account.id,
											force: force
										})
									);
									refresh();
								}, setter(remove))}
						>
							Yes, delete {account.name}
						</Button>
						<Button variant="outline" class="cursor-pointer" onclick={() => (confirmDelete = false)}>Cancel</Button>
					{:else}
						<Button variant="destructive" class="cursor-pointer" onclick={() => (confirmDelete = true)}>
							Delete Account
						</Button>
					{/if}
				</div>
				{#if remove.error}
					<Alert.Root variant="destructive">
						<AlertCircleIcon />
						<Alert.Title>Failed to delete account</Alert.Title>
						<Alert.Description>{remove.error}</Alert.Description>
					</Alert.Root>
				{/if}
			{/if}
		</div>
	</Card.Content>
</Card.Root>
