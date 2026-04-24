package migrate

import (
	"context"
	"strings"
	"testing"

	"github.com/trakrf/platform/backend/internal/buildinfo"
)

func TestRun_MissingPGURL(t *testing.T) {
	t.Setenv("PG_URL", "")

	err := Run(context.Background(), buildinfo.Info{Version: "test"})
	if err == nil {
		t.Fatal("expected error when PG_URL is empty, got nil")
	}
	if !strings.Contains(err.Error(), "PG_URL") {
		t.Errorf("expected error mentioning PG_URL, got: %v", err)
	}
}
