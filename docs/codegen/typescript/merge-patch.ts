// openapi-fetch + RFC 7396 merge-patch Content-Type helper for TrakRF.
//
// openapi-fetch is intentionally schema-agnostic at runtime, so it cannot tell
// which paths expect application/merge-patch+json. TrakRF's PATCH endpoints
// require that media type — without this middleware, every PATCH returns 415.
//
// Two equivalent entry points are provided. Pick whichever fits your build:
//
//   import createClient from "openapi-fetch";
//   import { mergePatchMiddleware } from "./merge-patch";
//   const client = createClient<paths>({ baseUrl });
//   client.use(mergePatchMiddleware);
//
//   // or, single import:
//   import { createTrakrfClient } from "./merge-patch";
//   const client = createTrakrfClient<paths>({ baseUrl });
//
// `paths` is the type emitted by `openapi-typescript` against the TrakRF spec.

import createClient, {
  type Client,
  type ClientOptions,
  type Middleware,
} from "openapi-fetch";

const MERGE_PATCH_CONTENT_TYPE = "application/merge-patch+json";

export const mergePatchMiddleware: Middleware = {
  async onRequest({ request }) {
    if (request.method === "PATCH") {
      request.headers.set("Content-Type", MERGE_PATCH_CONTENT_TYPE);
    }
    return request;
  },
};

export function createTrakrfClient<Paths extends {}>(
  options: ClientOptions,
): Client<Paths> {
  const client = createClient<Paths>(options);
  client.use(mergePatchMiddleware);
  return client;
}
