<script lang="ts">
	import OptionalSelect from '$lib/components/custom/OptionalSelect.svelte';
	import { Badge } from '$lib/components/shad/badge';
	import { Button } from '$lib/components/shad/button';
	import { Input } from '$lib/components/shad/input';
	import * as Select from '$lib/components/shad/select';
	import { scopes } from '$lib/scopes.svelte';
	import { RBACService } from '$lib/sdk/v1/admin/rbac/rbac_pb';
	import { RoleService } from '$lib/sdk/v1/admin/role/role_pb';
	import { SystemService } from '$lib/sdk/v1/admin/system/system_pb';
	import { UserService } from '$lib/sdk/v1/admin/user/user_pb';
	import globToRegex from 'glob-to-regexp';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import { ProviderService } from '$lib/sdk/v1/cloud/provider/provider_pb';
	import { AuthService } from '$lib/sdk/v1/auth/auth_pb';
	import { AccountService } from '$lib/sdk/v1/cloud/account/account_pb';
	import { DefinitionService } from '$lib/sdk/v1/cloud/definition/definition_pb';
	import { GameService } from '$lib/sdk/v1/play/game/game_pb';
	import { ChallengeService } from '$lib/sdk/v1/play/challenge/challenge_pb';
	import { TeamService } from '$lib/sdk/v1/play/team/team_pb';

	let { entries = $bindable() }: { entries: { scope: string; patterns: string }[] } = $props();

	const services = [
		AuthService,
		UserService,
		RoleService,
		RBACService,
		SystemService,
		ProviderService,
		AccountService,
		DefinitionService,
		GameService,
		ChallengeService,
		TeamService
	].map((svc) => ({
		name: svc.name,
		functions: svc.methods.map((method) => ({
			label: `${svc.name}/${method.name}`,
			procedure: `/${svc.typeName}/${method.name}`
		}))
	}));
	const functions = services.flatMap((svc) => svc.functions);

	// granted returns a human readable list of functions that are granted according to the specified patterns.
	function granted(patterns: string): string[] {
		const matched = [];
		for (const pattern of patterns.split(',')) {
			const patternRe = globToRegex(pattern);
			matched.push(...functions.filter((fn) => patternRe.test(fn.procedure)).map((fn) => fn.label));
		}
		return matched;
	}

	function selected(patterns: string): string[] {
		const parts = patterns.split(',');
		return functions.filter((fn) => parts.includes(fn.procedure)).map((fn) => fn.procedure);
	}

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
				<OptionalSelect
					bind:value={entry.scope}
					placeholder="Scope"
					suggestions={scopes.map((scope) => ({ id: scope, title: scope }))}
				/>
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
				<span class="text-muted-foreground text-xs">grants:</span>
				{#if granted(entry.patterns).length === functions.length}
					<Badge variant="secondary">every function</Badge>
				{:else}
					{#each granted(entry.patterns) as label (label)}
						<Badge variant="outline">{label}</Badge>
					{:else}
						<span class="text-muted-foreground text-xs italic">no known function</span>
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
