import type { SubmitState } from '$lib';

export let gameState: SubmitState = $state({ error: '', loading: true, forbidden: false });
