/**
 * Playwright globalSetup: refuse to start a run whose environment cannot pass it.
 *
 * Ticket: TRA-1190.
 *
 * Three separate unmet preconditions produced large, confident-looking failure
 * counts on one afternoon, and none of them named itself:
 *
 *   1. no backend running        → failures inside signupTestUser, triaged as
 *                                  "fixture rot" — a label that was then carried
 *                                  in a commit message as fact
 *   2. schema two migrations behind → 89+ failures, every one at exactly 11.3s,
 *                                  because login 500'd and every spec logs in
 *   3. dev server started as `pnpm vite` rather than `pnpm dev:bridge`
 *                                → 13 @hardware failures reading "Bridge server
 *                                  not ready", while the bridge was up, fresh
 *                                  and idle the whole time
 *
 * In each case the suite failed inside an assertion that named something else,
 * so debugging started at a healthy component. A uniform failure duration was
 * the only real tell, and it only reads as one if you already suspect a single
 * shared cause.
 *
 * This file checks the two preconditions that are cheap to check from outside a
 * browser and fails the whole run with the unmet one named. Case 3 is handled
 * where it is detectable — at the page, in helpers/connection.ts.
 *
 * Deliberately fatal rather than a warning. A warning scrolls past in the first
 * of 90 failures; the cost being avoided here is not a slow run, it is a wrong
 * conclusion recorded as a finding.
 */

/** Where the app will actually send its API calls. */
function apiBase(): string {
  const configured = process.env.VITE_API_URL;
  if (configured) return configured.replace(/\/+$/, '');
  const origin = (process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:5173').replace(/\/+$/, '');
  return `${origin}/api/v1`;
}

/**
 * /health.json sits at the server root, not under the API prefix — and the
 * dotted form is the one that resolves past the SPA catch-all.
 */
function healthUrl(): string {
  return `${apiBase().replace(/\/api\/v1$/, '')}/health.json`;
}

function fail(precondition: string, lines: string[]): never {
  const rule = '─'.repeat(72);
  throw new Error(
    [
      '',
      rule,
      `E2E PRECONDITION NOT MET: ${precondition}`,
      rule,
      ...lines,
      rule,
      'No specs were run. Fixing the above is not optional — a run started in',
      'this state produces failures that describe the environment, not the code,',
      'and they are indistinguishable from real defects after the fact.',
      '',
    ].join('\n')
  );
}

interface HealthPayload {
  status?: string;
  version?: string;
  schema?: {
    applied?: number;
    expected?: number;
    dirty?: boolean;
    pending?: string[];
  };
}

export default async function assertPreconditions(): Promise<void> {
  const url = healthUrl();

  let response: Response;
  try {
    response = await fetch(url, {
      signal: AbortSignal.timeout(5000),
      headers: { accept: 'application/json' },
    });
  } catch (error) {
    // Precondition 1. The failure this replaces happened inside signupTestUser,
    // where it looked like a broken fixture rather than an absent server.
    fail('the backend is not reachable', [
      `  tried:  ${url}`,
      `  error:  ${(error as Error).message}`,
      '',
      '  Start it with:  just dev          (database + migrations + backend)',
      '              or:  just backend dev  (backend only, against a running db)',
    ]);
  }

  let payload: HealthPayload = {};
  try {
    payload = (await response.json()) as HealthPayload;
  } catch {
    // A non-JSON body on this path almost always means the dev server answered
    // instead of the backend — an SPA index.html, served with a 200.
    fail('the health endpoint did not return JSON', [
      `  tried:  ${url}`,
      `  status: ${response.status}`,
      '',
      '  This usually means the request reached the vite dev server rather than',
      '  the backend. Set VITE_API_URL to the backend, e.g.',
      '    VITE_API_URL=http://localhost:8080/api/v1',
    ]);
  }

  // Precondition 2. The backend reports this about itself now (TRA-1190): it
  // compares the migration set embedded in the binary against the version the
  // database has applied, and refuses to look healthy when it is behind.
  if (payload.status === 'schema_behind') {
    const s = payload.schema ?? {};
    fail('the database schema is behind the backend', [
      `  applied:  ${s.applied}`,
      `  expected: ${s.expected}`,
      ...(s.pending?.length
        ? ['  unapplied migrations:', ...s.pending.map((m) => `    - ${m}`)]
        : []),
      '',
      '  Endpoints touching those migrations fail while this is true. Login is',
      '  usually the first, which fails every spec in the suite identically.',
      '',
      '  Fix:  just backend migrate',
    ]);
  }

  if (payload.status === 'schema_dirty') {
    fail('the migration ledger is dirty', [
      `  applied: ${payload.schema?.applied}`,
      '',
      '  A migration aborted partway, so the schema matches neither the old',
      '  shape nor the new one, and further migrations refuse to run.',
      '',
      '  For a local database, rebuilding is the cheapest repair:',
      '    docker compose down -v && just database up && just backend migrate',
    ]);
  }

  if (!response.ok) {
    fail('the backend is unhealthy', [
      `  tried:  ${url}`,
      `  status: ${response.status}`,
      `  body:   ${JSON.stringify(payload)}`,
    ]);
  }

  const schema = payload.schema;
  const version = payload.version ?? 'unknown';
  console.log(
    `[preflight] backend ${version} reachable at ${url}` +
      (schema ? `, schema ${schema.applied}/${schema.expected}` : '')
  );
}
