import { getSubject, Glue } from '$lib';
import { create } from '@bufbuild/protobuf';
import { GetRequestSchema as GetRoleRequestSchema } from './sdk/v1/admin/role/role_pb';
import { GetRequestSchema as GetUserRequestSchema } from './sdk/v1/admin/user/user_pb';

// scopes represents a global "helper" state describing the scopes of the users role.
// This is purely to assist the user in configuring an entity scope (may also be empty even if the user has access).
export const scopes: string[] = $state([]);

// loadScopes fetches the user / role and sets the scopes accordingly.
// It also removes self and admin scopes.
export async function loadScopes() {
	if (!getSubject()) return;
	const user = await Glue.user.get(create(GetUserRequestSchema, {}));
	const role = await Glue.role.get(
		create(GetRoleRequestSchema, {
			id: user.user?.role
		})
	);
	const suggestions = Object.keys(role.role?.permissions ?? []).filter(
		(scope) => scope !== 'self' && scope !== 'admin'
	);
	scopes.splice(0, scopes.length, ...suggestions);
}
