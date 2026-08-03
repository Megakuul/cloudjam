<script lang="ts">
	import { create, type Message } from '@bufbuild/protobuf';
	import { CreateRequestSchema, type CreateRequest } from '$lib/sdk/v1/admin/user/user_pb';
	import { Glue } from '$lib';
	import { UserSchema } from '$lib/sdk/v1/admin/user_pb';
	import {Input} from "$lib/components/shad/input";
	import {Button} from "$lib/components/shad/button";
	import * as Field from '$lib/components/shad/field';
	import { User } from '@lucide/svelte';

	let user = $state(
		create(UserSchema, {
			username: '',
			email: ''
		})
	);

	let validator = $derived(Glue.Validate(UserSchema, user));
	let validUsername = $derived(validator.violation.username !== undefined);
</script>

<h1>Create User</h1>

<form
	onsubmit={() => {
		user = user;
		console.log(user);
	}}
></form>

<Button variant="outline">Bodenlos</Button>

<Field.Field data-invalid={validUsername}>
	<Field.Label for="username">Username</Field.Label>
	<Input bind:value={user.username} id="username" type="text" placeholder="Enter your username" aria-invalid={validUsername} />
	<Field.Error>{validator.violation.username}</Field.Error>
</Field.Field>

