# Feature: Email Environment Indicator

## Origin
Linear issue TRA-279 - Sub-issue of TRA-282 (environment banner). Extends environment identification to email notifications.

## Outcome
All outbound emails clearly indicate the environment when sent from non-production, preventing users from confusing test invitations with real ones.

## User Story
As a TrakRF user receiving email notifications
I want to clearly see which environment the email came from
So that I don't accidentally accept test invitations or reset passwords on the wrong environment

## Context
**Discovery**: TRA-282 established the pattern of using `APP_ENV` to identify environments. The parent issue specifically called out: "Match UI labeling in system emails... reduces accidental cross-environment clicks from notifications."

**Current**: Email subjects use plain "TrakRF" branding with no environment indicator:
- `You've been invited to join {org} on TrakRF`
- `Reset your TrakRF password`

**Desired**: Non-production emails include environment in subject and body:
- `[TrakRF Preview] You've been invited to join {org}`
- `[TrakRF Staging] Reset your password`
- Production remains clean: `[TrakRF] You've been invited...`

## Technical Requirements

### Environment Detection
- **Variable**: `APP_ENV` (same as frontend banner)
- **Values**: `preview`, `staging`, `dev`, `production`, or empty
- **Behavior**:
  - If `production` or empty → use `[TrakRF]` prefix
  - Otherwise → use `[TrakRF {Env}]` prefix (capitalized)

### Email Subject Format
| Environment | Subject Prefix |
|-------------|----------------|
| production  | `[TrakRF]`     |
| (empty)     | `[TrakRF]`     |
| preview     | `[TrakRF Preview]` |
| staging     | `[TrakRF Staging]` |
| dev         | `[TrakRF Dev]` |

### Affected Emails
1. **Invitation Email** (`SendInvitationEmail`)
   - Current: `You've been invited to join {org} on TrakRF`
   - New: `[TrakRF {Env}] You've been invited to join {org}`

2. **Password Reset Email** (`SendPasswordResetEmail`)
   - Current: `Reset your TrakRF password`
   - New: `[TrakRF {Env}] Reset your password`

### Email Body Addition
Add environment notice in non-production emails:
```html
<p style="color: #6b7280; font-size: 12px;">
  This email was sent from the {Env} environment.
</p>
```

### Implementation Location
- File: `backend/internal/services/email/resend.go`
- Add helper function to get email prefix based on `APP_ENV`
- Update both `SendInvitationEmail` and `SendPasswordResetEmail`

## Code Example

```go
// getEmailPrefix returns the appropriate email subject prefix based on APP_ENV
func getEmailPrefix() string {
    env := os.Getenv("APP_ENV")
    if env == "" || env == "production" {
        return "[TrakRF]"
    }
    // Capitalize first letter
    return fmt.Sprintf("[TrakRF %s]", strings.Title(env))
}

// getEnvironmentNotice returns HTML notice for non-prod environments
func getEnvironmentNotice() string {
    env := os.Getenv("APP_ENV")
    if env == "" || env == "production" {
        return ""
    }
    return fmt.Sprintf(`<p style="color: #6b7280; font-size: 12px;">This email was sent from the %s environment.</p>`, strings.Title(env))
}
```

## Non-Goals
- Email template redesign (separate feature)
- Different sender addresses per environment
- Email routing based on environment

## Validation Criteria
- [ ] Invitation emails from preview show `[TrakRF Preview]` in subject
- [ ] Password reset emails from preview show `[TrakRF Preview]` in subject
- [ ] Production emails show `[TrakRF]` (no environment name)
- [ ] Non-production email bodies include environment notice
- [ ] Unit tests for `getEmailPrefix()` function
- [ ] Manual test: Send test invitation from preview environment

## Files to Modify
1. `backend/internal/services/email/resend.go` - Add prefix helper, update email functions

## References
- Linear: [TRA-279](https://linear.app/trakrf/issue/TRA-279)
- Parent: [TRA-282](https://linear.app/trakrf/issue/TRA-282) - Environment banner (completed)
- Pattern: Uses same `APP_ENV` variable as frontend banner
