// Runtime smoke for the merge-patch helper. Mocks fetch, exercises one PATCH
// and one POST through the wrapped client, asserts Content-Type behavior.
//
// Runs via `pnpm dlx tsx smoke-test.ts` against a generated schema sibling
// (the codegen-smoke CI workflow places `schema.d.ts` next to this file).

import type { paths } from "./schema";
import { createTrakrfClient } from "./merge-patch";

type Capture = { method: string; contentType: string | null };
const captured: Capture[] = [];

const mockFetch: typeof fetch = async (input, init) => {
  const req = new Request(input as RequestInfo, init);
  captured.push({
    method: req.method,
    contentType: req.headers.get("content-type"),
  });
  return new Response(JSON.stringify({ data: { id: 1, name: "mock" } }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
};

const client = createTrakrfClient<paths>({
  baseUrl: "https://example.invalid",
  fetch: mockFetch,
});

await client.POST("/api/v1/assets", {
  body: { external_key: "X", name: "n" },
});
await client.PATCH("/api/v1/assets/{asset_id}", {
  params: { path: { asset_id: 1 } },
  body: { name: "renamed" },
});

const [post, patch] = captured;
const failures: string[] = [];

if (post.method !== "POST") failures.push(`expected first call POST, got ${post.method}`);
if (post.contentType !== "application/json") {
  failures.push(`POST Content-Type should be application/json, got ${post.contentType}`);
}

if (patch.method !== "PATCH") failures.push(`expected second call PATCH, got ${patch.method}`);
if (patch.contentType !== "application/merge-patch+json") {
  failures.push(
    `PATCH Content-Type should be application/merge-patch+json, got ${patch.contentType}`,
  );
}

if (failures.length > 0) {
  console.error("merge-patch helper smoke FAILED:");
  for (const f of failures) console.error("  -", f);
  process.exit(1);
}

console.log("merge-patch helper smoke OK");
console.log("  POST  ->", post.contentType);
console.log("  PATCH ->", patch.contentType);
