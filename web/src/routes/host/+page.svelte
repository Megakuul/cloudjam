<script lang="ts">
	import * as Card from '$lib/components/shad/card';
	import GamepadIcon from '@lucide/svelte/icons/gamepad-2';
	import WandSparklesIcon from '@lucide/svelte/icons/wand-sparkles';
</script>

<svelte:head>
	<title>Host | CloudJam</title>
	<meta property="og:title" content="Host | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="/favicon.png" />
</svelte:head>

<div class="flex flex-col gap-4 items-center w-full">
	<h1 class="my-8 text-4xl opacity-80">Host</h1>
	<div class="grid gap-4 w-full md:grid-cols-2">
		<a href="/host/games/">
			<Card.Root class="h-full transition-all duration-200 cursor-pointer hover:bg-slate-50/5">
				<Card.Header>
					<GamepadIcon class="size-8" />
					<Card.Title class="text-2xl">Games</Card.Title>
					<Card.Description>Schedule game events</Card.Description>
				</Card.Header>
			</Card.Root>
		</a>
		<a href="/host/design/">
			<Card.Root class="h-full transition-all duration-200 cursor-pointer hover:bg-slate-50/5">
				<Card.Header>
					<WandSparklesIcon class="size-8" />
					<Card.Title class="text-2xl">Design</Card.Title>
					<Card.Description>Design and upload your challenges</Card.Description>
				</Card.Header>
			</Card.Root>
		</a>
	</div>
</div>
