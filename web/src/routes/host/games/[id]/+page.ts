import { Glue, Submit } from '$lib';
import { GetRequestSchema } from '$lib/sdk/v1/play/game/game_pb';
import type { Game } from '$lib/sdk/v1/play/game_pb';
import { create } from '@bufbuild/protobuf';
import type { PageLoad } from './$types';
import { gameState } from './state.svelte';

export const prerender = 'auto';
export const ssr = false;
export const trailingSlash = 'always';

export const load: PageLoad = async ({ params }) => {
	let game: Game | undefined;

	await Promise.race([
		Submit(async () => {
			game = (await Glue.game.get(create(GetRequestSchema, { id: params.id }))).game;
		}, gameState),
		new Promise((resolve) => {
			setTimeout(resolve, 500);
		})
	]);

	return {
		game
	};
};
