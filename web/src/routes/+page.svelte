<script lang="ts">
	import { Button, TextInput } from 'carbon-components-svelte';
	import { create, type Message } from '@bufbuild/protobuf';
	import { createValidator, type Violation } from '@bufbuild/protovalidate';
	import { CreateRequestSchema, type CreateRequest } from '$lib/sdk/v1/admin/user/user_pb';
	import type { GenMessage } from '@bufbuild/protobuf/codegenv2';
	import { UserSchema } from '$lib/sdk/v1/admin/user_pb';

	let user = $state(
		create(CreateRequestSchema, {
			ananas: {
				somearray: ['aas']
			}
		})
	);
	let violations: Violation[] = $derived.by(() => {
		console.log(protoValidator.validate(CreateRequestSchema, user).violations);
		return protoValidator.validate(CreateRequestSchema, user).violations ?? [];
	});

	function getViolation(violations: Violation[], fieldName: string): string {
		return (
			violations.find((v) => v.field[0].kind === 'field' && v.field[0].name === fieldName)
				?.message ?? ''
		);
	}
</script>

<h1>Create User</h1>

<form
	onsubmit={() => {
		user = user;
		console.log(user);
	}}
>
	<TextInput
		labelText="Username"
		bind:value={user.username}
		invalid={Boolean(validate(CreateRequestSchema, user).v.ananas?.v?.somemap)}
		invalidText={validate(CreateRequestSchema, user).v.username.v}
	/>

	<TextInput
		labelText="Username"
		bind:value={user.username}
		invalid={Boolean(getViolation(violations, 'username'))}
		invalidText={getViolation(violations, 'username')}
	/>

	<Button type="submit">Submit</Button>
</form>
