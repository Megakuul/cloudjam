// zstd compression for plugin binaries, backed by the reference libzstd compiled to wasm.
// The module is loaded lazily, it is only needed when a binary is actually uploaded.
let zstd: Promise<typeof import('@bokuweb/zstd-wasm')> | undefined;

function load(): Promise<typeof import('@bokuweb/zstd-wasm')> {
	zstd ??= import('@bokuweb/zstd-wasm')
		.then(async (module) => (await module.init(), module))
		.catch((err) => ((zstd = undefined), Promise.reject(err)));
	return zstd;
}

// compresses the data into a single zstd frame.
export async function compress(data: Uint8Array, level: number = 3): Promise<Uint8Array> {
	return (await load()).compress(data, level);
}
