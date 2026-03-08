import { jwtDecode } from 'jwt-decode';
import { create } from '@bufbuild/protobuf';
import {
	LoginRequestSchema,
	LogoutRequestSchema
} from '$lib/sdk/v1/manager/authentication/authentication_pb';
import { AuthenticationClient } from './client.svelte';
import { VerifierSchema, type Verifier } from '$lib/sdk/v1/manager/verifier_pb';

/**
 * Login to the system (usually needed if GetToken() fails with status 16 "UNAUTHENTICATED").
 * Provide a verifier with the email of the user followed by a second call with the received code.
 * @param verifier channel to send verification code OR verification code
 */
export async function Login(verifier: Verifier) {
	const response = await AuthenticationClient().login(
		create(LoginRequestSchema, {
			verifier: verifier,
			autoRefresh: true
		})
	);
	if (response.token) {
		localStorage.setItem('auth_token', response.token);
	}
}

/**
 * Logout from the system
 */
export async function Logout() {
	await AuthenticationClient().logout(create(LogoutRequestSchema, {}));
	ClearToken();
}

/**
 * Retrieves the auth token from localstore or fetch a new one from the refresh_token.
 * @returns auth token
 */
export async function GetToken(): Promise<string> {
	const token = localStorage.getItem('auth_token');
	if (token && (jwtDecode(token, {}).exp ?? 0) * 1000 > Date.now()) {
		return token;
	}
	const response = await AuthenticationClient().login(
		create(LoginRequestSchema, {
			verifier: create(VerifierSchema, {}),
			autoRefresh: true
		})
	);
	localStorage.setItem('auth_token', response.token);
	return response.token;
}

/**
 * Clears the auth token from localstore.
 */
export function ClearToken() {
	localStorage.removeItem('auth_token');
}
