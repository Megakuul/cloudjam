import { UserService } from '$lib/sdk/v1/admin/user/user_pb';
import type { DescService, Message } from '@bufbuild/protobuf';
import type { GenMessage } from '@bufbuild/protobuf/codegenv2';
import { createValidator, type Validator } from '@bufbuild/protovalidate';
import { createClient, type Client } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';

type Violation<T> = {
	[K in keyof T]?: NonNullable<T[K]> extends Object ? Violation<T[K]> : string;
};

// Glue provides the foundation glue. It adds generic helper functions that are useful for working with the proto bridge.
// It is enhanced with the service clients later in the createGlue constructor.
class Glue {
	constructor(private validator: Validator) {}

	// Validate provides a simplified helper function to validate a proto structure.
	// Works for most basic use cases, if you need ultra fine grained access to nested map / list types use the proto validator directly.
	// Returns a general validation error and fine grained violations per field.
	Validate<M extends Message>(
		schema: GenMessage<M>,
		message: M
	): { error: string; violation: Violation<M> } {
		const violator: Violation<M> = {};

		const result = this.validator.validate(schema, message);
		if (result.violations) {
			for (const violation of result.violations) {
				let current: any = violator;
				for (let i = 0; i < violation.field.length; i++) {
					const field = violation.field[i];
					if (field.kind === 'field') {
						if (typeof current !== 'object') current = {};
						current[field.name] = current[field.name] || violation.message;
						current = current[field.name];
					}
				}
			}
		}
		return { error: result.error?.message ?? '', violation: violator };
	}
}

export function createGlue<T extends Record<string, DescService>>(
	validator: Validator,
	url: string,
	services: T
) {
	const glue = new Glue(validator);

	const transport = createConnectTransport({
		baseUrl: url,
		interceptors: [
			(next) => async (req) => {
				req.header.set('authorization', await GetToken());
				return await next(req);
			}
		]
	});

	for (const [key, client] of Object.entries(services)) {
		(glue as any)[key] = createClient(client, transport);
	}

	return glue as Glue & { [K in keyof T]: Client<T[K]> };
}
