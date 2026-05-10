import { jwtDecode } from 'jwt-decode';
import { ConnectError, type Interceptor } from '@connectrpc/connect';
import { createGlue } from '@megakuul/glue-protocol';
import { AuthService } from './sdk/v1/auth/auth_pb';
import { goto } from '$app/navigation';
import { UserService } from './sdk/v1/admin/user/user_pb';

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
		user: UserService
	}
);

// Submit is a helper for the common "unerror->spin->call->unspin->error" flow
// in most basic submit functions. Returns a usable user error and writes the full error
// to a error bus (currently console.error() TBD).
export async function Submit(
	call: () => Promise<void>,
	setState: (err: string, load: boolean) => void
) {
	setState('', true);
	try {
		const res = await call();
		setState('', false);
		return res;
	} catch (e: any) {
		const error = ConnectError.from(e);
		// TODO: implement more sophisticated error notification system.
		console.error(error.message);
		setState(error.rawMessage, false);
	}
}

export function setToken(token: string) {
	localStorage.setItem('auth_token', token);
}

function getToken(): string {
	const token = localStorage.getItem('auth_token');
	if (token && (jwtDecode(token, {}).exp ?? 0) * 1000 > Date.now()) {
		return token;
	}
	goto('/login');
	return '';
}
