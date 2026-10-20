package outbox

import "github.com/trakrf/platform/backend/internal/storage"

// *storage.Storage must satisfy the narrow batch store surface; this fails
// the build, not a test run, when a storage signature drifts.
var _ batchStore = (*storage.Storage)(nil)
