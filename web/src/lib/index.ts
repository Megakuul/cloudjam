import { goto } from '$app/navigation';
import { create } from '@bufbuild/protobuf';
import { timestampDate, timestampFromDate, type Timestamp } from '@bufbuild/protobuf/wkt';
import { Code, ConnectError, type Interceptor } from '@connectrpc/connect';
import { createGlue } from '@megakuul/glue-protocol';
import { jwtDecode } from 'jwt-decode';
import { RBACService } from './sdk/v1/admin/rbac/rbac_pb';
import { RoleService } from './sdk/v1/admin/role/role_pb';
import { SystemService } from './sdk/v1/admin/system/system_pb';
import { GetRequestSchema, UserService } from './sdk/v1/admin/user/user_pb';
import type { User } from './sdk/v1/admin/user_pb';
import { AuthService } from './sdk/v1/auth/auth_pb';
import { AccountService } from './sdk/v1/cloud/account/account_pb';
import { DefinitionService } from './sdk/v1/cloud/definition/definition_pb';
import { ProviderService } from './sdk/v1/cloud/provider/provider_pb';
import { ChallengeService } from './sdk/v1/play/challenge/challenge_pb';
import { GameService } from './sdk/v1/play/game/game_pb';
import { TeamService } from './sdk/v1/play/team/team_pb';

export const Glue = createGlue(
	'/api',
	(_): Interceptor => {
		return (next) => async (req) => {
			req.header.set('authorization', getToken());
			return await next(req);
		};
	},
	{
		auth: AuthService,
		user: UserService,
		role: RoleService,
		rbac: RBACService,
		system: SystemService,
		provider: ProviderService,
		account: AccountService,
		definition: DefinitionService,
		game: GameService,
		team: TeamService,
		challenge: ChallengeService
	}
);

// Submit is a helper for the common "unerror->spin->call->unspin->error" flow
// in most basic submit functions. Returns a usable user error and writes the full error
// to a error bus (currently console.error() TBD).
export async function Submit(
	call: () => Promise<void>,
	setState: (err: string, load: boolean, forbidden: boolean) => void
) {
	setState('', true, false);
	try {
		const res = await call();
		setState('', false, false);
		return res;
	} catch (e: any) {
		const error = ConnectError.from(e);
		if (error.code === Code.PermissionDenied) {
			setState(error.rawMessage, false, true);
		} else {
			// TODO: implement more sophisticated error notification system.
			console.error(error.message);
			setState(error.rawMessage, false, false);
		}
	}
}

let self: User | undefined;

// getSelf returns the requesting user. The user service resolves the caller when no id is
// given, which is the only way to learn the own scope; the token carries just pub_id and email.
// Cached because it only changes on login. Returns undefined without self management access.
export async function getSelf(): Promise<User | undefined> {
	if (!self) {
		try {
			self = (await Glue.user.get(create(GetRequestSchema, {}))).user;
		} catch {
			return undefined;
		}
	}
	return self;
}

// zstd frame magic, used to tell an already compressed upload from a raw plugin.
const zstdMagic = [0x28, 0xb5, 0x2f, 0xfd];

// pluginBinary reads a challenge plugin and returns the zstd frame the runtime expects.
// zstdify's encoder throws on some inputs (larger plugins in particular), so a failure falls
// back to level 1, which stores the plugin in a valid frame instead of failing the upload.
// The codec is only pulled in when a plugin is actually uploaded.
export async function pluginBinary(file: File): Promise<Uint8Array> {
	const data = new Uint8Array(await file.arrayBuffer());
	if (zstdMagic.every((byte, index) => data[index] === byte)) return data;

	const { compress } = await import('zstdify');
	try {
		return compress(data, { level: 3 });
	} catch (e) {
		// TODO: implement more sophisticated error notification system.
		console.error(`failed to compress the plugin, storing it uncompressed: ${e}`);
		return compress(data, { level: 1 });
	}
}

// the definition hash is carried as a raw byte string, base64 makes the digest readable.
export function toDigest(hash: string): string {
	if (!hash) return '';
	return btoa(String.fromCharCode(...Uint8Array.from(hash, (char) => char.charCodeAt(0) & 0xff)));
}

// datetime-local inputs work on the local wall clock, protobuf timestamps on utc instants.
export function toLocalInput(timestamp?: Timestamp): string {
	const date = timestamp ? timestampDate(timestamp) : new Date();
	return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}

export function fromLocalInput(value: string): Timestamp {
	return timestampFromDate(new Date(value));
}

export function setToken(token: string) {
	localStorage.setItem('auth_token', token);
}

export function getEmail(): string {
	const token = localStorage.getItem('auth_token');
	if (token) {
		const decoded = jwtDecode(token, {});
		if ((decoded.exp ?? 0) * 1000 > Date.now()) return (decoded as any).email;
	}
	return '';
}

export function getPubId(): string {
	const token = localStorage.getItem('auth_token');
	if (token) {
		const decoded = jwtDecode(token, {});
		if ((decoded.exp ?? 0) * 1000 > Date.now()) return (decoded as any).pub_id;
	}
	return '';
}

function getToken(): string {
	const token = localStorage.getItem('auth_token');
	if (token && (jwtDecode(token, {}).exp ?? 0) * 1000 > Date.now()) {
		return token;
	}
	goto('/login');
	return '';
}

/**
 * Convert a longer string into initials of a set length
 * @param value The string to initialise
 * @param length Optional, a length to cap the initials ot
 * @returns A string that has been initialised
 */
export function toShortInitials(value: string, length: number = 2) {
	if (value.length <= length) {
		return value.toUpperCase();
	}

	return value
		.split(' ')
		.join('')
		.substring(0, length - 1)
		.toUpperCase();
}
