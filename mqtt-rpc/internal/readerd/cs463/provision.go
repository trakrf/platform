package cs463

// Reconcile orchestration: verify the pre-created CloudServer + Operation Profile
// exist, then converge the four owned entities (Data Format, Trigger, Resultant
// Action, Event) to their golden definitions. List-then-add-or-mod, idempotent.
// Operates on the entityOps seam so it is tested without an HTTP round-trip.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
)

// ErrMissingCloudServer is the sentinel for the one PERMANENT commissioning
// prerequisite: the golden CloudServer (NameMQTTServer) must be hand-crafted on
// the reader before the daemon can build the golden chain. The daemon fatal-exits
// on this (errors.Is) — unlike transient reconcile errors (reader busy, broker
// down, GlassFish booting), which are served-and-retried. The naming is
// deliberately exact/opinionated; see MissingCloudServerError for the operator hint.
var ErrMissingCloudServer = errors.New("cs463: required golden CloudServer not found")

// MissingCloudServerError reports the missing golden CloudServer plus the
// server_ids actually present on the reader, so a near-miss (typo / extra spaces,
// e.g. "TrakRF mqtt-rpc MQTT   Server") is obvious in the fatal log. It satisfies
// errors.Is(err, ErrMissingCloudServer).
type MissingCloudServerError struct {
	Want    string
	Present []string
}

func (e *MissingCloudServerError) Error() string {
	return fmt.Sprintf("cs463: required CloudServer %q not found — hand-craft it before commissioning (TRA-1002); CloudServers present: %v", e.Want, e.Present)
}

func (e *MissingCloudServerError) Is(target error) bool { return target == ErrMissingCloudServer }

// entityOps is the slice of reader operations the reconcile needs. *Client
// satisfies it; tests inject a fake.
type entityOps interface {
	ListServer(ctx context.Context, session string) (map[string]EntityRow, error)
	ListProfileIDs(ctx context.Context, session string) (map[string]bool, error)
	ListDataFormat(ctx context.Context, session string) (map[string]EntityRow, error)
	ListTriggeringLogic(ctx context.Context, session string) (map[string]EntityRow, error)
	ListResultantAction(ctx context.Context, session string) (map[string]EntityRow, error)
	ListEvent(ctx context.Context, session string) (map[string]EntityRow, error)
	AddDataFormat(ctx context.Context, session string, p url.Values) error
	ModDataFormat(ctx context.Context, session string, p url.Values) error
	AddTriggeringLogic(ctx context.Context, session string, p url.Values) error
	ModTriggeringLogic(ctx context.Context, session string, p url.Values) error
	AddResultantAction(ctx context.Context, session string, p url.Values) error
	ModResultantAction(ctx context.Context, session string, p url.Values) error
	AddEvent(ctx context.Context, session string, p url.Values) error
	ModEvent(ctx context.Context, session string, p url.Values) error
}

var _ entityOps = (*Client)(nil)

// entitySpec binds one golden entity's read/drift/write closures so reconcile can
// treat all four uniformly.
type entitySpec struct {
	name   string
	list   func(ctx context.Context, session string) (map[string]EntityRow, error)
	drift  func(EntityRow) bool
	params func() url.Values
	add    func(ctx context.Context, session string, p url.Values) error
	mod    func(ctx context.Context, session string, p url.Values) error
}

// specsFor builds the ordered entity specs: dependencies (format, trigger) before
// dependents (action references server+format, event references profile+trigger+action).
func specsFor(r entityOps, antennaCount int) []entitySpec {
	return []entitySpec{
		{
			name: NameDataFormat, list: r.ListDataFormat, drift: dataFormatDrift,
			params: goldenDataFormatParams, add: r.AddDataFormat, mod: r.ModDataFormat,
		},
		{
			name: NameTrigger, list: r.ListTriggeringLogic,
			drift:  func(row EntityRow) bool { return triggerDrift(row, antennaCount) },
			params: func() url.Values { return goldenTriggerParams(antennaCount) },
			add:    r.AddTriggeringLogic, mod: r.ModTriggeringLogic,
		},
		{
			name: NameAction, list: r.ListResultantAction, drift: actionDrift,
			params: goldenActionParams, add: r.AddResultantAction, mod: r.ModResultantAction,
		},
		{
			name: NameEvent, list: r.ListEvent, drift: eventDrift,
			params: goldenEventParams, add: r.AddEvent, mod: r.ModEvent,
		},
	}
}

// verifyServer fails (loudly) if the hand-crafted CloudServer the golden chain
// references is absent. The server is the one prereq the daemon cannot create (it
// needs the broker secret + TLS cert). The Operation Profile, by contrast, IS created
// by the daemon (see Adapter.ensureProfile), so it is not verified here.
func verifyServer(ctx context.Context, session string, r entityOps) error {
	servers, err := r.ListServer(ctx, session)
	if err != nil {
		return fmt.Errorf("cs463: reconcile listServer: %w", err)
	}
	if _, ok := servers[NameMQTTServer]; !ok {
		present := make([]string, 0, len(servers))
		for id := range servers {
			present = append(present, id)
		}
		sort.Strings(present)
		return &MissingCloudServerError{Want: NameMQTTServer, Present: present}
	}
	return nil
}

// reconcileGolden converges the four owned entities to golden: absent -> add,
// drifted -> mod, matching -> no-op. Returns whether anything was written, so the
// caller re-arms the event only when a change occurred. Aborts on first error
// (partial-failure policy: surfaced loudly, bench-revisitable).
func reconcileGolden(ctx context.Context, session string, r entityOps, antennaCount int) (changed bool, err error) {
	for _, s := range specsFor(r, antennaCount) {
		rows, err := s.list(ctx, session)
		if err != nil {
			return changed, fmt.Errorf("cs463: reconcile %s list: %w", s.name, err)
		}
		cur, exists := rows[s.name]
		switch {
		case !exists:
			if err := s.add(ctx, session, s.params()); err != nil {
				return changed, fmt.Errorf("cs463: reconcile %s add: %w", s.name, err)
			}
			changed = true
		case s.drift(cur):
			if err := s.mod(ctx, session, s.params()); err != nil {
				return changed, fmt.Errorf("cs463: reconcile %s mod: %w", s.name, err)
			}
			changed = true
		}
	}
	return changed, nil
}
