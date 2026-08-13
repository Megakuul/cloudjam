<script lang="ts">
	import ScopeInput from '$lib/components/custom/ScopeInput.svelte';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import { Input } from '$lib/components/shad/input';
	import * as Select from '$lib/components/shad/select';
	import { RBACService } from '$lib/sdk/v1/admin/rbac/rbac_pb';
	import { RoleService } from '$lib/sdk/v1/admin/role/role_pb';
	import { SystemService } from '$lib/sdk/v1/admin/system/system_pb';
	import { UserService } from '$lib/sdk/v1/admin/user/user_pb';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';

	// entries hold the role permission map as editable list;
	// every entry grants the patterns (comma separated globs matched
	// against the rpc procedure name) to subjects within the scope.
	let { entries = $bindable(), scopes }: { entries: { scope: string; patterns: string }[]; scopes: string[] } =
		$props();

	// functions of all rbac protected services (from the generated sdk), used for
	// suggestions; a picked function is granted by its exact procedure name.
	const services = [UserService, RoleService, RBACService, SystemService].map((svc) => ({
		name: svc.name,
		functions: svc.methods.map((method) => ({
			label: `${svc.name}/${method.name}`,
			procedure: `/${svc.typeName}/${method.name}`
		}))
	}));
	const functions = services.flatMap((svc) => svc.functions);

	// granted resolves the functions a pattern list grants by compiling the globs
	// like the backend does ('/' separated, '*' stops at separators, '**' does not).
	function granted(patterns: string): string[] {
		const regexes = patterns
			.split(',')
			.filter((pattern) => pattern)
			.map(
				(pattern) =>
					new RegExp(
						`^${pattern
							.replace(/[.+?^${}()|[\]\\]/g, '\\$&')
							.replaceAll('**', '\u0000')
							.replaceAll('*', '[^/]*')
							.replaceAll('\u0000', '.*')}$`
					)
			);
		return functions.filter((fn) => regexes.some((regex) => regex.test(fn.procedure))).map((fn) => fn.label);
	}

	// selected extracts the patterns that map directly to a suggested function.
	function selected(patterns: string): string[] {
		const parts = patterns.split(',');
		return functions.filter((fn) => parts.includes(fn.procedure)).map((fn) => fn.procedure);
	}

	// apply writes the picked function procedures back, keeping manually written globs.
	function apply(entry: { scope: string; patterns: string }, picked: string[]) {
		const custom = entry.patterns
			.split(',')
			.filter((pattern) => pattern && !functions.some((fn) => fn.procedure === pattern));
		entry.patterns = [...custom, ...picked].join(',');
	}
</script>

<div class="flex flex-col gap-3">
	{#each entries as entry, i (i)}
		<div class="flex flex-col gap-1">
			<div class="flex flex-row items-center gap-2">
				<ScopeInput class="max-w-48" bind:value={entry.scope} {scopes} placeholder="Scope" />
				<Select.Root type="multiple" value={selected(entry.patterns)} onValueChange={(picked) => apply(entry, picked)}>
					<Select.Trigger class="w-48 cursor-pointer">
						{selected(entry.patterns).length} function(s) picked
					</Select.Trigger>
					<Select.Content>
						{#each services as service (service.name)}
							<Select.Group>
								<Select.GroupHeading>{service.name}</Select.GroupHeading>
								{#each service.functions as fn (fn.procedure)}
									<Select.Item value={fn.procedure} label={fn.label}>{fn.label}</Select.Item>
								{/each}
							</Select.Group>
						{/each}
					</Select.Content>
				</Select.Root>
				<Input
					class="max-w-96 font-mono"
					bind:value={entry.patterns}
					placeholder="Patterns (e.g. /v1.admin.user.UserService/*)"
				/>
				<Button
					variant="ghost"
					size="icon"
					title="Remove permission"
					class="cursor-pointer"
					onclick={() => (entries = entries.filter((_, x) => x !== i))}
				>
					<Trash2Icon />
				</Button>
			</div>
			<div class="flex flex-row flex-wrap items-center gap-1">
				<span class="text-xs text-muted-foreground">grants:</span>
				{#if granted(entry.patterns).length === functions.length}
					<Badge variant="secondary">every function</Badge>
				{:else}
					{#each granted(entry.patterns) as label (label)}
						<Badge variant="outline">{label}</Badge>
					{:else}
						<span class="text-xs text-muted-foreground italic">no known function</span>
					{/each}
				{/if}
			</div>
		</div>
	{/each}
	<Button
		variant="outline"
		size="sm"
		class="cursor-pointer self-start"
		onclick={() => (entries = [...entries, { scope: '', patterns: '' }])}
	>
		<PlusIcon /> Add Permission
	</Button>
</div>
