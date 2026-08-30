import adapter from '@sveltejs/adapter-static';
import { mdsvex } from 'mdsvex';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		adapter: adapter({
			fallback: 'index.html',
			precompress: false,
			strict: true
		})
	},
	preprocess: [mdsvex()],
	extensions: ['.svelte', '.svx']
};

export default config;
