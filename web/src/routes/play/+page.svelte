<script lang="ts">
	import { goto } from '$app/navigation';
	import { Button } from '$lib/components/shad/button';
	import * as Card from '$lib/components/shad/card';
	import { Input } from '$lib/components/shad/input';

	let code = $state('');
</script>

<svelte:head>
	<title>Play | CloudJam</title>
	<meta property="og:title" content="Play | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex w-full flex-col gap-4">
	<div class="flex w-full justify-center">
		<Card.Root class="mt-[10%] w-96">
			<Card.Header>
				<Card.Title class="text-2xl">Join a Game</Card.Title>
				<Card.Description>Enter the game code your host handed you.</Card.Description>
			</Card.Header>
			<Card.Content>
				<form
					class="flex flex-col gap-4"
					onsubmit={(e) => {
						e.preventDefault();
						goto(`/play/${code}`);
					}}
				>
					<Input bind:value={code} placeholder="Game code" />
					<Button type="submit" class="cursor-pointer" disabled={!code.trim()}>Join</Button>
				</form>
			</Card.Content>
		</Card.Root>
	</div>
</div>
