<script lang="ts">
	import { Button } from '$lib/components/shad/button';
	import { Input } from '$lib/components/shad/input';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import EyeOffIcon from '@lucide/svelte/icons/eye-off';

	// Provider credentials travel as an opaque json blob. The aws provider deserializes it into
	// the shape below (see internal/provider/aws/aws.go Credentials), so the form edits those
	// fields directly instead of asking for raw json.
	type credentials = { endpoint: string; region: string; access_key: string; secret_key: string };

	let { value = $bindable() }: { value: string } = $props();

	const uid = $props.id();

	function parse(raw: string): credentials {
		try {
			const parsed = JSON.parse(raw);
			return {
				endpoint: parsed.endpoint ?? '',
				region: parsed.region ?? '',
				access_key: parsed.access_key ?? '',
				secret_key: parsed.secret_key ?? ''
			};
		} catch {
			return { endpoint: '', region: '', access_key: '', secret_key: '' };
		}
	}

	// the form owns the parsed shape, the raw json is only regenerated from it.
	// svelte-ignore state_referenced_locally
	let creds = $state(parse(value));
	let reveal = $state(false);

	$effect(() => {
		if (!creds.endpoint && !creds.region && !creds.access_key && !creds.secret_key) {
			value = '';
		} else {
			value = JSON.stringify({
				endpoint: creds.endpoint,
				region: creds.region,
				access_key: creds.access_key,
				secret_key: creds.secret_key
			});
		}
	});
</script>

<div class="grid gap-4 md:grid-cols-2">
	<div class="flex flex-col gap-1">
		<label for="{uid}-region" class="text-sm">Region</label>
		<Input id="{uid}-region" bind:value={creds.region} placeholder="us-east-1" />
		<p class="text-muted-foreground text-xs">Region the organization api is called in.</p>
	</div>
	<div class="flex flex-col gap-1">
		<label for="{uid}-endpoint" class="text-sm">Endpoint</label>
		<Input id="{uid}-endpoint" bind:value={creds.endpoint} placeholder="https://localhost:4566 (optional)" />
		<p class="text-muted-foreground text-xs">Only for emulators like fakecloud, leave empty for real AWS.</p>
	</div>
	<div class="flex flex-col gap-1">
		<label for="{uid}-access-key" class="text-sm">Access Key ID</label>
		<Input id="{uid}-access-key" bind:value={creds.access_key} placeholder="AKIA..." />
	</div>
	<div class="flex flex-col gap-1">
		<label for="{uid}-secret-key" class="text-sm">Secret Access Key</label>
		<div class="flex flex-row items-center gap-2">
			<Input
				id="{uid}-secret-key"
				type={reveal ? 'text' : 'password'}
				bind:value={creds.secret_key}
				placeholder="Secret access key of the user"
			/>
			<Button
				variant="outline"
				size="icon"
				title={reveal ? 'Hide' : 'Reveal'}
				class="cursor-pointer"
				onclick={() => (reveal = !reveal)}
			>
				{#if reveal}
					<EyeOffIcon />
				{:else}
					<EyeIcon />
				{/if}
			</Button>
		</div>
	</div>
</div>
<p class="text-muted-foreground text-xs">
	The credentials of the organization management account. CloudJam calls the organizations api with them and assumes its
	roles into the sandbox accounts, so they need organization and role assumption permissions.
</p>
