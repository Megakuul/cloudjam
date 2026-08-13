// the definition hash is carried as a raw byte string, base64 makes the digest readable.
export function toDigest(hash: string): string {
	if (!hash) return '';
	return btoa(String.fromCharCode(...Uint8Array.from(hash, (char) => char.charCodeAt(0) & 0xff)));
}
