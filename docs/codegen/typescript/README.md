# `openapi-fetch` merge-patch helper for TrakRF

`openapi-fetch` is a thin, spec-agnostic client — it has no runtime knowledge
of which paths expect `application/merge-patch+json`, so it defaults
`Content-Type` to `application/json` on every method. TrakRF's `PATCH`
endpoints follow RFC 7396 and require `application/merge-patch+json`; without
the right header every `PATCH` returns `415 unsupported_media_type`.

This snippet is a ~30-line middleware that sets the correct `Content-Type` on
`PATCH` requests and leaves every other method at `openapi-fetch` defaults.

Python's `openapi-generator` already emits a client that handles merge-patch
correctly; this snippet exists to bring the TypeScript path to parity.

## Use it

Copy `merge-patch.ts` into your codegen project (alongside the `schema.d.ts`
emitted by `openapi-typescript`), then either:

```ts
import createClient from "openapi-fetch";
import { mergePatchMiddleware } from "./merge-patch";
import type { paths } from "./schema";

const client = createClient<paths>({ baseUrl: "https://app.trakrf.id" });
client.use(mergePatchMiddleware);
```

…or use the single-call wrapper that registers the middleware for you:

```ts
import { createTrakrfClient } from "./merge-patch";
import type { paths } from "./schema";

const client = createTrakrfClient<paths>({ baseUrl: "https://app.trakrf.id" });

await client.PATCH("/api/v1/assets/{id}", {
  params: { path: { id: 42 } },
  body: { name: "renamed" },
});
// → Content-Type: application/merge-patch+json on the wire, automatically
```

## Generate the schema

```bash
pnpm dlx openapi-typescript https://app.trakrf.id/api/openapi.yaml \
  -o ./schema.d.ts
```

## Verify

`smoke-test.ts` is a runnable check — it mocks `fetch`, drives one `POST` and
one `PATCH` through the wrapped client, and asserts:

- `POST` keeps `Content-Type: application/json`
- `PATCH` is rewritten to `application/merge-patch+json`

The codegen-smoke GitHub Actions workflow runs it on every PR that touches the
OpenAPI spec or this directory.
