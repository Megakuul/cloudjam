<script lang="ts">
	import { create, type Message } from '@bufbuild/protobuf';
	import { CreateRequestSchema, type CreateRequest } from '$lib/sdk/v1/admin/user/user_pb';
	import { Glue } from '$lib';
	import { UserSchema } from '$lib/sdk/v1/admin/user_pb';
	import {Input} from "$lib/components/shad/input";
	import {Button} from "$lib/components/ui/button";

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

<Button variant="outline">Bodenlos</Button>

<!--Linus, what is this?-->
<Input placeholder="" />

<Input type="text" label="Bombaclad" placeholder="Enter your bombaclad" />


<!--<TextField-->
<!--	bind:value={user.username}-->
<!--	label="Bombaclad"-->
<!--	placeholder="Enter your bombaclad"-->
<!--	error={Glue.Validate(UserSchema, user).violation.username}-->
<!--/>-->
