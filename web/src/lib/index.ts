import { goto } from '$app/navigation';
import { Code, ConnectError, type Interceptor } from '@connectrpc/connect';
import { createGlue } from '@megakuul/glue-protocol';
import { jwtDecode } from 'jwt-decode';
import { RBACService } from './sdk/v1/admin/rbac/rbac_pb';
import { RoleService } from './sdk/v1/admin/role/role_pb';
import { SystemService } from './sdk/v1/admin/system/system_pb';
import { UserService } from './sdk/v1/admin/user/user_pb';
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

// SubmitState is the object that holds the state of the Submit.
// It is recommended to create a state per operation to avoid a full page loading spinner if a small action failed.
// Usually you want to instantiate this as a $state to make the properties reactive proxies.
export type SubmitState = { error: string; loading: boolean; forbidden: boolean };

// Submit is a helper for the common "unerror->spin->call->unspin->error" flow
// in most basic submit functions. Returns a usable user error and writes the full error
// to a error bus (currently console.error() TBD).
export async function Submit(call: () => Promise<void>, state: SubmitState) {
	((state.loading = true), (state.error = ''), (state.forbidden = false));
	try {
		const res = await call();
		((state.loading = false), (state.error = ''), (state.forbidden = false));
		return res;
	} catch (e: any) {
		const error = ConnectError.from(e);
		if (error.code === Code.PermissionDenied) {
			((state.loading = false), (state.error = error.rawMessage), (state.forbidden = true));
		} else {
			// TODO: implement more sophisticated error notification system.
			console.error(error.message);
			((state.loading = false), (state.error = error.rawMessage), (state.forbidden = false));
		}
	}
}

export function setToken(token: string) {
	localStorage.setItem('auth_token', token);
}

export function getSubject(): string {
	const token = localStorage.getItem('auth_token');
	if (token) {
		const decoded = jwtDecode(token, {});
		if ((decoded.exp ?? 0) * 1000 > Date.now()) return (decoded as any).sub;
	}
	return '';
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
