<script lang="ts">
	import {
		mdiTrashCan,
		mdiFormTextboxPassword,
		mdiEmailEditOutline,
		mdiCheckCircleOutline,
		mdiCloseCircleOutline,
		mdiPencil,
		mdiLogout
	} from '@mdi/js';
</script>

<svelte:head>
	<title>Admin | CloudJam</title>
	<meta property="og:title" content="Admin | CloudJam" />
	<meta property="og:type" content="website" />
	<meta property="og:image" content="favicon.png" />
</svelte:head>

<div class="flex flex-col gap-4 justify-center items-center w-full">
	{#if error}
		<div class="p-2 m-2 w-full rounded-sm border-[0.05rem] border-red-600/80 bg-red-600/10">
			{error}
		</div>
	{/if}
</div>
