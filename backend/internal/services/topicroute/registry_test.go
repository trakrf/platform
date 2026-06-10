package topicroute

import (
	"context"
	"io"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/storage"
)

func testLogger() zerolog.Logger { return zerolog.New(io.Discard) }

type fakeLister struct {
	m   map[string]storage.ScanRoute
	err error
}

func (f *fakeLister) ListScanTopics(context.Context) (map[string]storage.ScanRoute, error) {
	return f.m, f.err
}

type fakeMgr struct {
	subs   []string
	unsubs []string
}

func (f *fakeMgr) Subscribe(t string)   { f.subs = append(f.subs, t) }
func (f *fakeMgr) Unsubscribe(t string) { f.unsubs = append(f.unsubs, t) }

func TestReconcile_AddsAndLooksUp(t *testing.T) {
	l := &fakeLister{m: map[string]storage.ScanRoute{
		"org-a/dock-1/reads": {OrgID: 1, ScanDeviceID: 10, DeviceType: "csl_cs463"},
	}}
	mgr := &fakeMgr{}
	r := NewRegistry(l, testLogger())
	r.SetManager(mgr)

	require.NoError(t, r.Reconcile(context.Background()))

	got, ok := r.Lookup("org-a/dock-1/reads")
	assert.True(t, ok)
	assert.Equal(t, 10, got.ScanDeviceID)
	assert.Equal(t, 1, got.OrgID)
	assert.Equal(t, []string{"org-a/dock-1/reads"}, mgr.subs)
	assert.Empty(t, mgr.unsubs)
}

func TestReconcile_RemovesGoneTopics(t *testing.T) {
	l := &fakeLister{m: map[string]storage.ScanRoute{"org-a/d/reads": {OrgID: 1, ScanDeviceID: 1}}}
	mgr := &fakeMgr{}
	r := NewRegistry(l, testLogger())
	r.SetManager(mgr)
	require.NoError(t, r.Reconcile(context.Background()))

	l.m = map[string]storage.ScanRoute{} // device deleted
	require.NoError(t, r.Reconcile(context.Background()))

	_, ok := r.Lookup("org-a/d/reads")
	assert.False(t, ok)
	assert.Equal(t, []string{"org-a/d/reads"}, mgr.unsubs)
}

func TestReconcile_NoDeltaIsQuiet(t *testing.T) {
	l := &fakeLister{m: map[string]storage.ScanRoute{"o/d/reads": {ScanDeviceID: 5}}}
	mgr := &fakeMgr{}
	r := NewRegistry(l, testLogger())
	r.SetManager(mgr)
	require.NoError(t, r.Reconcile(context.Background()))
	require.NoError(t, r.Reconcile(context.Background())) // identical second pass

	assert.Equal(t, []string{"o/d/reads"}, mgr.subs) // subscribed exactly once
	assert.Empty(t, mgr.unsubs)
}

func TestReconcile_NoManagerIsMapOnly(t *testing.T) {
	l := &fakeLister{m: map[string]storage.ScanRoute{"o/d/reads": {ScanDeviceID: 5}}}
	r := NewRegistry(l, testLogger()) // no SetManager
	require.NoError(t, r.Reconcile(context.Background()))
	_, ok := r.Lookup("o/d/reads")
	assert.True(t, ok)
}

func TestTopicsSnapshot(t *testing.T) {
	l := &fakeLister{m: map[string]storage.ScanRoute{"a/x/reads": {}, "b/y/reads": {}}}
	r := NewRegistry(l, testLogger())
	require.NoError(t, r.Reconcile(context.Background()))
	assert.ElementsMatch(t, []string{"a/x/reads", "b/y/reads"}, r.Topics())
}
