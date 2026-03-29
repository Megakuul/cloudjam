<script lang="ts">
	import { create, type Message } from '@bufbuild/protobuf';
	import { CreateRequestSchema, type CreateRequest } from '$lib/sdk/v1/admin/user/user_pb';
	import { TextField, ToggleButton } from 'svelte-ux';
	import { Input } from 'svelte-ux';
	import { Glue } from '$lib';
	import { UserSchema } from '$lib/sdk/v1/admin/user_pb';

	let user = $state(
		create(UserSchema, {
			username: ''
		})
	);
</script>

<h1>Create User</h1>

<form
	onsubmit={() => {
		user = user;
		console.log(user);
	}}
></form>

<ToggleButton buttonPlacement="after">Bodenlos</ToggleButton>

<Input placeholder="" />

<TextField
	bind:value={user.username}
	label="Bombaclad"
	placeholder="Enter your bombaclad"
	error={Glue.Validate(UserSchema, user).violation.username}
/>
