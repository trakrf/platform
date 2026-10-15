package subscriptionnotice

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/storage"
)

type fakeStore struct {
	due     []storage.SubscriptionNotice
	claimed map[int]bool // orgID → already claimed by someone else
	admins  map[int][]string
	claims  []int
}

func (f *fakeStore) ListDueSubscriptionNotices(context.Context) ([]storage.SubscriptionNotice, error) {
	return f.due, nil
}

func (f *fakeStore) ClaimSubscriptionNotice(_ context.Context, orgID int, _ storage.SubscriptionNoticeKind, _ time.Time) (bool, error) {
	f.claims = append(f.claims, orgID)
	return !f.claimed[orgID], nil
}

func (f *fakeStore) ListOrgAdminEmails(_ context.Context, orgID int) ([]string, error) {
	return f.admins[orgID], nil
}

type sent struct{ to, kind, org string }

type fakeSender struct {
	sent []sent
	fail map[string]bool
}

func (f *fakeSender) SendSubscriptionNotice(to, kind, org string, _, _ time.Time) error {
	if f.fail[to] {
		return errors.New("boom")
	}
	f.sent = append(f.sent, sent{to, kind, org})
	return nil
}

func TestRunOnce_SendsToEveryAdminOfEachDueOrg(t *testing.T) {
	now := time.Now()
	st := &fakeStore{
		due: []storage.SubscriptionNotice{
			{OrgID: 1, OrgName: "One", Kind: storage.NoticeTMinus14, ExpiresAt: now, CutoffAt: now},
			{OrgID: 2, OrgName: "Two", Kind: storage.NoticeCutoff, ExpiresAt: now, CutoffAt: now},
		},
		admins: map[int][]string{1: {"a@one.test", "b@one.test"}, 2: {"a@two.test"}},
	}
	snd := &fakeSender{}
	n := New(st, snd, zerolog.New(io.Discard))

	got, err := n.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, got)
	assert.Equal(t, []sent{
		{"a@one.test", "t_minus_14", "One"},
		{"b@one.test", "t_minus_14", "One"},
		{"a@two.test", "cutoff", "Two"},
	}, snd.sent)
}

func TestRunOnce_SkipsNoticesClaimedElsewhere(t *testing.T) {
	st := &fakeStore{
		due:     []storage.SubscriptionNotice{{OrgID: 1, Kind: storage.NoticeExpired}},
		claimed: map[int]bool{1: true},
		admins:  map[int][]string{1: {"a@one.test"}},
	}
	snd := &fakeSender{}
	got, err := New(st, snd, zerolog.New(io.Discard)).RunOnce(context.Background())
	require.NoError(t, err)
	assert.Zero(t, got)
	assert.Empty(t, snd.sent)
}

func TestRunOnce_OneFailedSendDoesNotStopTheRest(t *testing.T) {
	st := &fakeStore{
		due:    []storage.SubscriptionNotice{{OrgID: 1, Kind: storage.NoticeTMinus3}},
		admins: map[int][]string{1: {"bad@one.test", "good@one.test"}},
	}
	snd := &fakeSender{fail: map[string]bool{"bad@one.test": true}}
	got, err := New(st, snd, zerolog.New(io.Discard)).RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, got)
	assert.Equal(t, []sent{{"good@one.test", "t_minus_3", ""}}, snd.sent)
}
