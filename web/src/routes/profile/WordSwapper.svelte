<script lang="ts">
	import { onMount } from 'svelte';
	import { Gooey } from 'svelte-ux';
	import { circIn, circOut } from 'svelte/easing';
	import { blur } from 'svelte/transition';

	let { description } = $props();

	let currentWord = $state('');

	onMount(() => {
		const interval = setInterval(() => {
			if (description) {
				const words = description.split(' ');
				if (words.length < 1) {
					currentWord = '';
					return;
				}
				words.push('');
				// designed this weird to be resilient if the desc is changed.
				const currentIdx = words.indexOf(currentWord);
				if (currentIdx === -1 || currentIdx + 1 >= words.length) currentWord = words[0];
				else currentWord = words[currentIdx + 1];
			}
		}, 1000);
		return () => clearInterval(interval);
	});
</script>

<Gooey blur={4} alphaPixel={255} alphaShift={-144}>
	<div class="grid place-items-center mt-48 w-full text-7xl font-bold text-center grid-stack">
		{#key currentWord}
			<span
				in:blur={{ amount: '10px', duration: 1000, easing: circOut }}
				out:blur={{ amount: '100px', duration: 1000, easing: circIn }}
			>
				{currentWord.slice(0, 15)}
			</span>
		{/key}
	</div>
</Gooey>
