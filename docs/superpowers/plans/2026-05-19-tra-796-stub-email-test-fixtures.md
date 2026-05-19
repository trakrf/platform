# TRA-796: Stub Email Sends to RFC 2606 Test Fixture Domains — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop calling Resend when the recipient is on an RFC 2606 reserved domain so e2e fixtures don't burn the 100/day free-tier quota.

**Architecture:** Add a small domain-classifier helper in the email package. Each public `Send*Email` method short-circuits at the top when the recipient is reserved — logs an info line and returns `nil` without invoking the Resend client. No interface seam, no env flag — the gate is the domain itself, which is reserved by RFC and applies in every environment.

**Tech Stack:** Go, `github.com/resend/resend-go/v2`, `github.com/rs/zerolog/log` (used elsewhere in the backend), Go's standard `testing` package + table-driven tests (pattern already established in `resend_test.go`).

**Linear ticket:** [TRA-796](https://linear.app/trakrf/issue/TRA-796/stub-email-sends-to-rfc-2606-test-fixture-domains)

---

## File Structure

- **Modify:** `backend/internal/services/email/resend.go`
  - Add unexported helper `isReservedTestRecipient(addr string) bool`
  - Add gate at top of `SendInvitationEmail` and `SendPasswordResetEmail`
- **Modify:** `backend/internal/services/email/resend_test.go`
  - Add `TestIsReservedTestRecipient` (table-driven)
  - Add `TestSendInvitationEmail_StubsReservedDomain` and `TestSendPasswordResetEmail_StubsReservedDomain` — verify no-op + nil return when recipient is reserved, using an obviously-invalid `RESEND_API_KEY` to prove no network call happens

No new files. All work lives inside the email package.

---

## Reserved-Domain Rules

The classifier matches the recipient's domain (everything after the last `@`, lowercased) against:

1. Exact match: `example.com`, `example.net`, `example.org`
2. Suffix match: `.test`, `.invalid`, `.example`

Anything else (including `trakrf.id`, `gmail.com`, customer domains) is **not** reserved and flows through to Resend normally.

If the address has no `@` or is empty, treat it as **not reserved** — let Resend reject it the way it would today; we're not in the input-validation business here.

---

## Task 1: Add the reserved-domain classifier (TDD)

**Files:**
- Modify: `backend/internal/services/email/resend.go`
- Test: `backend/internal/services/email/resend_test.go`

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/services/email/resend_test.go`:

```go
func TestIsReservedTestRecipient(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		expected bool
	}{
		{"example.com", "fixture@example.com", true},
		{"example.net", "fixture@example.net", true},
		{"example.org", "fixture@example.org", true},
		{".test TLD", "fixture@foo.test", true},
		{".invalid TLD", "fixture@foo.invalid", true},
		{".example TLD", "fixture@foo.example", true},
		{"case-insensitive domain", "Fixture@Example.COM", true},
		{"subdomain of example.com", "fixture@mail.example.com", true},
		{"trakrf.id", "user@trakrf.id", false},
		{"gmail", "user@gmail.com", false},
		{"gmail alias", "miks2u+t2@gmail.com", false},
		{"customer domain", "user@acme.co", false},
		{"empty", "", false},
		{"no at-sign", "not-an-email", false},
		{"trailing whitespace", "fixture@example.com  ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isReservedTestRecipient(tt.addr); got != tt.expected {
				t.Errorf("isReservedTestRecipient(%q) = %v, want %v", tt.addr, got, tt.expected)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run from project root:

```bash
just backend test ./internal/services/email/...
```

Expected: FAIL — `undefined: isReservedTestRecipient`.

- [ ] **Step 3: Implement the classifier**

Add to `backend/internal/services/email/resend.go`, immediately after the imports block (before `type Client struct`):

```go
// reservedTestDomains are RFC 2606 / RFC 6761 addresses reserved for documentation
// and testing. No real user can own one, so we never attempt to send to them —
// this prevents e2e fixtures from burning Resend quota.
var reservedTestDomains = map[string]struct{}{
	"example.com": {},
	"example.net": {},
	"example.org": {},
}

var reservedTestSuffixes = []string{".test", ".invalid", ".example"}

func isReservedTestRecipient(addr string) bool {
	at := strings.LastIndex(addr, "@")
	if at < 0 || at == len(addr)-1 {
		return false
	}
	domain := strings.ToLower(strings.TrimSpace(addr[at+1:]))
	if _, ok := reservedTestDomains[domain]; ok {
		return true
	}
	for _, s := range reservedTestSuffixes {
		if strings.HasSuffix(domain, s) {
			return true
		}
	}
	// Subdomain of an exact-match reserved domain (e.g. mail.example.com)
	for d := range reservedTestDomains {
		if strings.HasSuffix(domain, "."+d) {
			return true
		}
	}
	return false
}
```

Then add `"strings"` to the import block. The final imports should be:

```go
import (
	"fmt"
	"os"
	"strings"

	"github.com/resend/resend-go/v2"
	"github.com/rs/zerolog/log"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)
```

(The `log` import isn't used yet — Task 2 will add the call sites. Goimports / go vet will flag it as unused if we add it now, so add it together with the call sites in Task 2 instead. Update the imports here to add only `"strings"`.)

- [ ] **Step 4: Run test to verify it passes**

```bash
just backend test ./internal/services/email/...
```

Expected: PASS — all sub-cases of `TestIsReservedTestRecipient` green, existing `TestGetEmailPrefix` / `TestGetEnvironmentNotice` still green.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/services/email/resend.go backend/internal/services/email/resend_test.go
git commit -m "feat(email): add RFC 2606 reserved-domain classifier (TRA-796)"
```

---

## Task 2: Gate `SendInvitationEmail` and `SendPasswordResetEmail` (TDD)

**Files:**
- Modify: `backend/internal/services/email/resend.go:50-71` (`SendPasswordResetEmail`)
- Modify: `backend/internal/services/email/resend.go:75-97` (`SendInvitationEmail`)
- Test: `backend/internal/services/email/resend_test.go`

The test strategy: set `RESEND_API_KEY` to an obviously-invalid value, call the Send method with an `@example.com` recipient, and assert `err == nil`. Without the gate, `resend.Client.Emails.Send` would attempt an HTTP call and either error or hang — so a clean `nil` return proves the stub fired. We also assert it errors (or at least doesn't return nil) for a non-reserved recipient, to prove the gate isn't catching everything.

> Note on the negative case: the call to `gmail.com` actually performs a network call against the Resend API. The point isn't to verify the API rejection — it's to confirm the function doesn't short-circuit. Skip this assertion in short-mode (`testing.Short()`) so `go test -short` stays offline-safe.

- [ ] **Step 1: Write the failing tests**

Append to `backend/internal/services/email/resend_test.go`:

```go
func TestSendInvitationEmail_StubsReservedDomain(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "invalid-key-should-never-be-used")
	c := NewClient()

	if err := c.SendInvitationEmail(
		"fixture@example.com",
		"Test Org",
		"Inviter Name",
		"member",
		"token-xyz",
		"https://app.preview.trakrf.id",
	); err != nil {
		t.Fatalf("expected nil error for reserved recipient, got %v", err)
	}
}

func TestSendPasswordResetEmail_StubsReservedDomain(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "invalid-key-should-never-be-used")
	c := NewClient()

	if err := c.SendPasswordResetEmail(
		"fixture@example.com",
		"https://app.preview.trakrf.id/#reset-password",
		"token-xyz",
	); err != nil {
		t.Fatalf("expected nil error for reserved recipient, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
just backend test ./internal/services/email/...
```

Expected: FAIL — both new tests return a non-nil error from `resend.Client.Emails.Send` because the gate doesn't exist yet (or hang / take a long time on the network call). If they hang for more than a few seconds, hit Ctrl+C and proceed; the point is to confirm we're not short-circuiting yet.

- [ ] **Step 3: Add the gate to both Send methods**

In `backend/internal/services/email/resend.go`, modify the import block to add `"github.com/rs/zerolog/log"`. Final imports:

```go
import (
	"fmt"
	"os"
	"strings"

	"github.com/resend/resend-go/v2"
	"github.com/rs/zerolog/log"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)
```

Then at the top of `SendPasswordResetEmail`, immediately after the `fullResetURL` line, insert the gate:

```go
func (c *Client) SendPasswordResetEmail(toEmail, resetURL, token string) error {
	fullResetURL := fmt.Sprintf("%s?token=%s", resetURL, token)

	if isReservedTestRecipient(toEmail) {
		log.Info().
			Str("to", toEmail).
			Str("kind", "password_reset").
			Str("app_env", os.Getenv("APP_ENV")).
			Msg("email send stubbed: reserved test-fixture recipient")
		return nil
	}

	_, err := c.client.Emails.Send(&resend.SendEmailRequest{
		// ... unchanged ...
	})
```

And at the top of `SendInvitationEmail`, immediately after the `acceptURL` line:

```go
func (c *Client) SendInvitationEmail(toEmail, orgName, inviterName, role, token, baseURL string) error {
	acceptURL := fmt.Sprintf("%s/#accept-invite?token=%s", baseURL, token)

	if isReservedTestRecipient(toEmail) {
		log.Info().
			Str("to", toEmail).
			Str("kind", "invitation").
			Str("org", orgName).
			Str("app_env", os.Getenv("APP_ENV")).
			Msg("email send stubbed: reserved test-fixture recipient")
		return nil
	}

	_, err := c.client.Emails.Send(&resend.SendEmailRequest{
		// ... unchanged ...
	})
```

Use `_ = acceptURL` is **not** needed — `acceptURL` is still referenced inside the unchanged `Html` block below. Same for `fullResetURL` in the password-reset case.

- [ ] **Step 4: Run tests to verify they pass**

```bash
just backend test ./internal/services/email/...
```

Expected: PASS — both stub tests now return `nil` immediately. `TestIsReservedTestRecipient`, `TestGetEmailPrefix`, `TestGetEnvironmentNotice` all still green.

- [ ] **Step 5: Vet and lint**

```bash
just backend vet
just backend lint
```

Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/services/email/resend.go backend/internal/services/email/resend_test.go
git commit -m "feat(email): skip Resend sends to reserved test-fixture domains (TRA-796)"
```

---

## Task 3: Push branch and open PR

**Files:** none (git/gh only).

- [ ] **Step 1: Push branch**

```bash
git push -u origin fix/tra-796-stub-email-test-fixtures
```

(Branch was created at the start by `EnterWorktree` with that name.)

- [ ] **Step 2: Open PR**

```bash
gh pr create --title "fix(email): stub Resend sends to reserved test-fixture domains (TRA-796)" --body "$(cat <<'EOF'
## Summary
- Adds `isReservedTestRecipient` classifier matching RFC 2606 / RFC 6761 reserved domains (`example.com|.net|.org`, `*.test`, `*.invalid`, `*.example`, and subdomains of `example.com|.net|.org`)
- Short-circuits `SendInvitationEmail` and `SendPasswordResetEmail` for reserved recipients — logs at info level and returns `nil` without calling Resend
- Applies in every environment (prod included); RFC 2606 addresses are reserved by definition, no real user owns one

## Why
Preview e2e exercises the full invite / password-reset flow per scenario. Each pass burns ~15–20 Resend sends on `@example.com` fixtures; we're on the 100/day free tier and started hitting the cap. Resend has been stable since integration — no value in exercising the actual outbound send in e2e.

## Test plan
- [x] `just backend test ./internal/services/email/...` — unit tests for classifier + both Send methods green
- [ ] Post-merge smoke (manual): invite `miks2u+t2@gmail.com` to a test org on prod, confirm email arrives
- [ ] Post-merge: tail preview backend logs during one e2e invite scenario, confirm `email send stubbed` log line fires
- [ ] Resend dashboard: confirm send count stops climbing during CI runs

Closes TRA-796
EOF
)"
```

- [ ] **Step 3: Confirm CI starts**

```bash
gh pr checks --watch
```

Wait for the first round of checks to register. Don't need to wait for them all to complete here — that's the user's call.

---

## Self-Review

**Spec coverage** — every acceptance criterion in TRA-796 maps to a task:
- "Domain checker recognizes …" → Task 1
- "SendInvitationEmail/SendPasswordResetEmail return nil and don't call Resend" → Task 2
- "Preview e2e run produces zero Resend sends; log lines confirm" → covered by the PR's manual test-plan checkbox (can't be unit-tested)
- "Real recipients on trakrf.id and customer domains continue to send normally" → covered by `TestIsReservedTestRecipient`'s negative cases + post-merge gmail invite

**Placeholder scan** — no TBDs, every code step shows the exact code, every command shows expected output.

**Type consistency** — function name `isReservedTestRecipient` is the same across the test, the implementation, and both call sites. Log field keys (`to`, `kind`, `app_env`, `org`) are consistent across the two call sites.
