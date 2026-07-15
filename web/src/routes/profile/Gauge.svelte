<script lang="ts">
	import { Tilt } from 'svelte-ux';
	import Gauge from 'svelte-gauge';

	let { center, inner, outer, title, scale } = $props();

	let max = $derived(Math.max(Number(outer) + 5, 12 * scale));
</script>

<Tilt maxRotation={8} class="rounded-full transition duration-500 hover:scale-105">
	<div
		class="pointer-events-none relative rounded-full border-[0.05rem] border-primary/20 bg-neutral/5 p-2 transition ease-out select-none"
	>
		<Gauge
			width={300}
			stop={max}
			labelsCentered={true}
			labels={Array.from(Array(Math.round((max + 1) / scale)), (_, i) => (i * scale).toString())}
			startAngle={45}
			stopAngle={360 - 45}
			stroke={10}
			value={Number(outer)}
			color={'var(--color-primary)'}
			class="font-bold text-primary"
		>
			<Gauge
				labels={Array.from(Array(Math.round((max + 1) / scale)), (_, i) => (i * scale).toString())}
				stop={max}
				startAngle={45}
				stopAngle={360 - 45}
				stroke={10}
				displayValue={Math.round(Number(center)).toString()}
				value={Number(inner)}
				color={'var(--color-primary)'}
				class="brightness-125"
			></Gauge>
			<h1 class="absolute bottom-2 left-1/2 translate-x-[-50%] text-2xl">{title}</h1>
		</Gauge>
	</div>
</Tilt>
