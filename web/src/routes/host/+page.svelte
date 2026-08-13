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

<div class="flex w-full flex-col items-center gap-4">
	<h1 class="my-8 text-4xl opacity-80">Host</h1>
	<div class="grid w-full gap-4 md:grid-cols-2">
		<a href="/host/games/">
			<Card.Root class="h-full cursor-pointer transition-all duration-200 hover:bg-slate-50/5">
				<Card.Header>
					<GamepadIcon class="size-8" />
					<Card.Title class="text-2xl">Games</Card.Title>
					<Card.Description>
						Schedule your gamedays. Teams and challenges are managed inside the game they belong to.
					</Card.Description>
				</Card.Header>
			</Card.Root>
		</a>
		<a href="/host/design/">
			<Card.Root class="h-full cursor-pointer transition-all duration-200 hover:bg-slate-50/5">
				<Card.Header>
					<WandSparklesIcon class="size-8" />
					<Card.Title class="text-2xl">Design</Card.Title>
					<Card.Description>Write and version the challenge plugins your games hand out.</Card.Description>
				</Card.Header>
			</Card.Root>
		</a>
	</div>
</div>
