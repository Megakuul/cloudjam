import { jwtDecode } from 'jwt-decode';
import type { Interceptor } from '@connectrpc/connect';
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

function getToken(): string {
	const token = localStorage.getItem('auth_token');
	if (token && (jwtDecode(token, {}).exp ?? 0) * 1000 > Date.now()) {
		return token;
	}
	goto('/login?reason=logged%20out');
	return '';
}
