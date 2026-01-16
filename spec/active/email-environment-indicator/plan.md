# Implementation Plan: Email Environment Indicator

## Overview
Add environment indicators to outbound emails so users can distinguish test emails from production.

**Pattern Reference**: `frontend/src/components/EnvironmentBanner.tsx:7` - same logic: show indicator for non-prod/non-production, hide otherwise.

## Task Breakdown

### Task 1: Add golang.org/x/text dependency
**File**: `backend/go.mod`

Add direct dependency (currently indirect):
```bash
cd backend && go get golang.org/x/text
```

**Validation**: `go mod tidy` runs without errors

---

### Task 2: Create helper functions
**File**: `backend/internal/services/email/resend.go`

Add imports at top (after existing imports):
```go
import (
    "fmt"
    "os"

    "github.com/resend/resend-go/v2"
    "golang.org/x/text/cases"
    "golang.org/x/text/language"
)
```

Add helper functions before `SendPasswordResetEmail`:

```go
// getEmailPrefix returns the appropriate email subject prefix based on APP_ENV.
// Production/empty returns "[TrakRF]", non-prod returns "[TrakRF Preview]" etc.
func getEmailPrefix() string {
    env := os.Getenv("APP_ENV")
    if env == "" || env == "production" || env == "prod" {
        return "[TrakRF]"
    }
    // Title case the environment name
    caser := cases.Title(language.English)
    return fmt.Sprintf("[TrakRF %s]", caser.String(env))
}

// getEnvironmentNotice returns an HTML notice for non-production environments.
// Returns empty string for production/empty.
func getEnvironmentNotice() string {
    env := os.Getenv("APP_ENV")
    if env == "" || env == "production" || env == "prod" {
        return ""
    }
    caser := cases.Title(language.English)
    return fmt.Sprintf(`<p style="color: #6b7280; font-size: 12px;">This email was sent from the %s environment.</p>`, caser.String(env))
}
```

**Validation**: `go build ./...` succeeds

---

### Task 3: Update SendPasswordResetEmail
**File**: `backend/internal/services/email/resend.go`

Change subject line at line 31 from:
```go
Subject: "Reset your TrakRF password",
```
To:
```go
Subject: fmt.Sprintf("%s Reset your password", getEmailPrefix()),
```

Add environment notice to HTML body (before closing backtick at line 37):
```go
Html: fmt.Sprintf(`
    <h2>Reset your password</h2>
    <p>Click the link below to reset your TrakRF password. This link expires in 24 hours.</p>
    <p><a href="%s">Reset Password</a></p>
    <p>If you didn't request this, you can safely ignore this email.</p>
    %s
`, fullResetURL, getEnvironmentNotice()),
```

**Validation**: `go build ./...` succeeds

---

### Task 4: Update SendInvitationEmail
**File**: `backend/internal/services/email/resend.go`

Change subject line at line 55 from:
```go
Subject: fmt.Sprintf("You've been invited to join %s on TrakRF", orgName),
```
To:
```go
Subject: fmt.Sprintf("%s You've been invited to join %s", getEmailPrefix(), orgName),
```

Add environment notice to HTML body (before closing backtick at line 62):
```go
Html: fmt.Sprintf(`
    <h2>You've been invited to %s</h2>
    <p>%s has invited you to join %s as a %s on TrakRF.</p>
    <p><a href="%s">Accept Invitation</a></p>
    <p>This invitation expires in 7 days.</p>
    <p>If you don't have a TrakRF account yet, you'll be prompted to create one.</p>
    %s
`, orgName, inviterName, orgName, role, acceptURL, getEnvironmentNotice()),
```

**Validation**: `go build ./...` succeeds

---

### Task 5: Add unit tests for helper functions
**File**: `backend/internal/services/email/resend_test.go` (new file)

```go
package email

import (
    "os"
    "testing"
)

func TestGetEmailPrefix(t *testing.T) {
    tests := []struct {
        name     string
        appEnv   string
        expected string
    }{
        {"empty env", "", "[TrakRF]"},
        {"production", "production", "[TrakRF]"},
        {"prod", "prod", "[TrakRF]"},
        {"preview", "preview", "[TrakRF Preview]"},
        {"staging", "staging", "[TrakRF Staging]"},
        {"dev", "dev", "[TrakRF Dev]"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            os.Setenv("APP_ENV", tt.appEnv)
            defer os.Unsetenv("APP_ENV")

            result := getEmailPrefix()
            if result != tt.expected {
                t.Errorf("getEmailPrefix() = %q, want %q", result, tt.expected)
            }
        })
    }
}

func TestGetEnvironmentNotice(t *testing.T) {
    tests := []struct {
        name        string
        appEnv      string
        shouldBeEmpty bool
        contains    string
    }{
        {"empty env", "", true, ""},
        {"production", "production", true, ""},
        {"prod", "prod", true, ""},
        {"preview", "preview", false, "Preview environment"},
        {"staging", "staging", false, "Staging environment"},
        {"dev", "dev", false, "Dev environment"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            os.Setenv("APP_ENV", tt.appEnv)
            defer os.Unsetenv("APP_ENV")

            result := getEnvironmentNotice()
            if tt.shouldBeEmpty && result != "" {
                t.Errorf("getEnvironmentNotice() = %q, want empty", result)
            }
            if !tt.shouldBeEmpty && result == "" {
                t.Errorf("getEnvironmentNotice() = empty, want non-empty containing %q", tt.contains)
            }
            if !tt.shouldBeEmpty && !strings.Contains(result, tt.contains) {
                t.Errorf("getEnvironmentNotice() = %q, want containing %q", result, tt.contains)
            }
        })
    }
}
```

Note: Add `"strings"` to imports.

**Validation**: `go test ./internal/services/email/...` passes

---

## Summary

| Task | File | Description |
|------|------|-------------|
| 1 | go.mod | Add golang.org/x/text direct dependency |
| 2 | resend.go | Add getEmailPrefix() and getEnvironmentNotice() helpers |
| 3 | resend.go | Update SendPasswordResetEmail subject and body |
| 4 | resend.go | Update SendInvitationEmail subject and body |
| 5 | resend_test.go | Add unit tests for helper functions |

## Validation Commands

```bash
cd backend
go mod tidy
go build ./...
go test ./internal/services/email/...
just validate
```

## Success Criteria
- [x] Helper functions use `golang.org/x/text/cases` for proper title casing
- [x] Production/empty APP_ENV shows `[TrakRF]` prefix (user choice: keep for consistency)
- [x] Non-production emails include environment in subject
- [x] Non-production email bodies include environment notice
- [x] All unit tests pass
