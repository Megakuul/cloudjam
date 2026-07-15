import { goto } from '$app/navigation';
import { Code, ConnectError, type Interceptor } from '@connectrpc/connect';
import { createGlue } from '@megakuul/glue-protocol';
import { jwtDecode } from 'jwt-decode';
import { RBACService } from './sdk/v1/admin/rbac/rbac_pb';
import { RoleService } from './sdk/v1/admin/role/role_pb';
import { SystemService } from './sdk/v1/admin/system/system_pb';
import { UserService } from './sdk/v1/admin/user/user_pb';
import { AuthService } from './sdk/v1/auth/auth_pb';

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
		system: SystemService
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

export function setToken(token: string) {
	localStorage.setItem('auth_token', token);
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
