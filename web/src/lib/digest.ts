// converts the provided raw sha256 hash into a human readable format.
export function toDigest(hash: Uint8Array): string {
	if (!hash) return '';
	return `sha256:${Array.from(hash, (b) => b.toString(16).padStart(2, '0')).join('')}`;
}
