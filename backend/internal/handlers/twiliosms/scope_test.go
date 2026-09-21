package twiliosms

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// An account-scoped signing key also authenticates other Messaging Services.
// Conflicting signed scope must never be persisted under this service's ID.
func TestCallbacks_RejectConflictingProviderScope(t *testing.T) {
	for _, endpoint := range []string{"status", "inbound"} {
		for _, tc := range []struct {
			name, account, service string
			want                   int
		}{
			{"configured scope", "AC123", "MG123", 204},
			{"different account", "ACother", "MG123", 400},
			{"different service", "AC123", "MGother", 400},
			{"empty account", "", "MG123", 400},
			{"empty service", "AC123", "", 400},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				form := url.Values{
					"MessageSid": {"SM123"}, "MessageStatus": {"delivered"},
					"Body": {"STOP"}, "From": {"+15550001111"}, "To": {"+15550002222"},
					"AccountSid": {tc.account}, "MessagingServiceSid": {tc.service},
				}
				statusConsumer := &statusConsumer{}
				keywordConsumer := &inboundConsumer{}
				rec := httptest.NewRecorder()
				if endpoint == "status" {
					newStatusTestHandler(t, statusConsumer, time.Time{}).Status(rec, signedStatusRequest(t, form))
				} else {
					newInboundTestHandler(t, keywordConsumer, time.Time{}).Inbound(rec, signedInboundRequest(t, form))
				}
				require.Equal(t, tc.want, rec.Code)
				if tc.want != http.StatusNoContent {
					require.Empty(t, statusConsumer.statuses)
					require.Empty(t, keywordConsumer.keywords)
				}
			})
		}
	}
}
