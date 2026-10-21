package routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/notification/phone"
	"github.com/trakrf/platform/backend/internal/storage"
)

// Store is the storage the planner reads and writes. *storage.Storage
// satisfies it, and it also satisfies what outbox.EnqueueBatchTx needs.
type Store interface {
	OrgIsEntitled(ctx context.Context, orgID int) (bool, error)
	GetAlarmOccurrence(ctx context.Context, orgID, id int) (notificationevent.Occurrence, bool, error)
	GetLocationChangeOccurrence(ctx context.Context, orgID, id int) (notificationevent.Occurrence, bool, error)
	LookupMoveNames(ctx context.Context, orgID int, assetIDs, locationIDs []int) (map[int]storage.AssetIdentity, map[int]string, error)
	ListSubscribersForAsset(ctx context.Context, orgID, assetID int) ([]notificationrecipient.Subscriber, error)
	IsSuppressed(ctx context.Context, orgID int, ch notificationdelivery.Channel, address string) (bool, error)
	WithOrgTx(ctx context.Context, orgID int, fn func(tx pgx.Tx) error) error
	InsertNotificationEventIfAbsentTx(ctx context.Context, tx pgx.Tx, orgID int, e notificationevent.NotificationEvent) (int, bool, error)
	InsertNotificationDeliveryIfAbsentTx(ctx context.Context, tx pgx.Tx, orgID int, d notificationdelivery.NotificationDelivery) (int64, bool, error)
	SetNotificationDeliveryRiverJobIDTx(ctx context.Context, tx pgx.Tx, orgID int, id, riverJobID int64) error
}

var _ Store = (*storage.Storage)(nil)

// Deps is everything a DeliveryPlanner needs; all but Log are required.
type Deps struct {
	Store         Store
	Jobs          outbox.JobInserter
	Renderer      Renderer
	Metrics       *Metrics
	OutboxMetrics *outbox.Metrics
	PhoneRegion   string
	Log           zerolog.Logger
}

// RecipientOutcome is what happened to one subscriber. DeliveryID is set only
// for a message that was enqueued or found already enqueued.
type RecipientOutcome struct {
	RecipientID int
	Channel     notificationdelivery.Channel
	DeliveryID  string
	Outcome     Outcome
}

// Result reports one Plan. Outcome is set when the whole event ended one way
// (unentitled, duplicate, asset deleted, no subscribers); otherwise it is
// empty and Recipients carries the outcomes.
type Result struct {
	EventID    int
	Outcome    Outcome
	Recipients []RecipientOutcome
}

// sourceSpec is everything that differs between an alarm and a move.
type sourceSpec struct {
	load  func(ctx context.Context, orgID, id int) (notificationevent.Occurrence, bool, error)
	setID func(e *notificationevent.NotificationEvent, id int)
}

// DeliveryPlanner turns a trigger into an event, its deliveries and their
// send jobs, all in one transaction.
type DeliveryPlanner struct {
	store         Store
	jobs          outbox.JobInserter
	renderer      Renderer
	metrics       *Metrics
	outboxMetrics *outbox.Metrics
	region        string
	log           zerolog.Logger
	sources       map[notificationevent.Source]sourceSpec
}

// NewDeliveryPlanner fails if any dependency is missing or the phone region
// is not one the phone package knows.
func NewDeliveryPlanner(d Deps) (*DeliveryPlanner, error) {
	switch {
	case d.Store == nil:
		return nil, errors.New("routing: planner store is required")
	case d.Jobs == nil:
		return nil, errors.New("routing: planner job inserter is required")
	case d.Renderer == nil:
		return nil, errors.New("routing: planner renderer is required")
	case d.Metrics == nil:
		return nil, errors.New("routing: planner metrics are required")
	case d.OutboxMetrics == nil:
		return nil, errors.New("routing: planner outbox metrics are required")
	}
	if err := phone.ValidateRegion(d.PhoneRegion); err != nil {
		return nil, fmt.Errorf("routing: planner phone region: %w", err)
	}
	return &DeliveryPlanner{
		store: d.Store, jobs: d.Jobs, renderer: d.Renderer,
		metrics: d.Metrics, outboxMetrics: d.OutboxMetrics,
		region: d.PhoneRegion, log: d.Log,
		sources: map[notificationevent.Source]sourceSpec{
			notificationevent.SourceAlarm: {
				load:  d.Store.GetAlarmOccurrence,
				setID: func(e *notificationevent.NotificationEvent, id int) { e.AlarmEventID = &id },
			},
			notificationevent.SourceLocationChange: {
				load:  d.Store.GetLocationChangeOccurrence,
				setID: func(e *notificationevent.NotificationEvent, id int) { e.LocationChangeID = &id },
			},
		},
	}, nil
}

// planned pairs a message with the recipient slot it belongs to.
type planned struct {
	slot int
	msg  DeliveryMessage
}

// scheduled is what the transaction wrote.
type scheduled struct {
	eventID   int
	duplicate bool // the event already existed; nothing else was written
	results   []outbox.BatchResult
}

// Plan registers the event and enqueues one delivery per eligible subscriber.
// A returned error other than ErrInvalidTrigger is worth retrying: nothing
// was written.
func (p *DeliveryPlanner) Plan(ctx context.Context, t Trigger) (Result, error) {
	if err := t.Validate(); err != nil {
		source := notificationevent.Source("unknown")
		if _, ok := p.sources[t.Source]; ok {
			source = t.Source
		}
		p.metrics.RecordOutcome(source, OutcomeInvalidTrigger)
		p.log.Error().Int("org_id", t.OrgID).Str("source", string(source)).Int("source_id", t.SourceID).
			Msg("notification trigger rejected")
		return Result{}, err
	}
	log := p.log.With().Int("org_id", t.OrgID).Str("source", string(t.Source)).Int("source_id", t.SourceID).Logger()

	res, err := p.plan(ctx, t, log)
	if err != nil {
		outcome := OutcomeFailed
		if errors.Is(err, ErrInvalidTrigger) {
			outcome = OutcomeInvalidTrigger
		}
		p.metrics.RecordOutcome(t.Source, outcome)
		log.Error().Err(err).Msg("notification planning failed")
		return Result{}, err
	}
	return res, nil
}

func (p *DeliveryPlanner) plan(ctx context.Context, t Trigger, log zerolog.Logger) (Result, error) {
	skip, err := outbox.CheckEntitlement(ctx, p.store, t.OrgID)
	if err != nil {
		return Result{}, err
	}
	if skip {
		return p.finish(log, t, Result{Outcome: OutcomeSkippedUnentitled}), nil
	}

	occ, err := p.loadSource(ctx, t)
	if err != nil {
		return Result{}, err
	}
	log = log.With().Int("asset_id", occ.AssetID).Logger()

	data, assetFound, err := p.loadNames(ctx, t.OrgID, occ)
	if err != nil {
		return Result{}, err
	}
	if !assetFound {
		sch, err := p.schedule(ctx, t, occ, nil)
		if err != nil {
			return Result{}, err
		}
		return p.finish(log, t, p.record(sch, nil, nil, OutcomeSkippedAssetDeleted)), nil
	}

	subs, err := p.store.ListSubscribersForAsset(ctx, t.OrgID, occ.AssetID)
	if err != nil {
		return Result{}, fmt.Errorf("routing: list subscribers for asset %d: %w", occ.AssetID, err)
	}
	cands, err := filterEligible(ctx, subs, p.region, func(ctx context.Context, ch notificationdelivery.Channel, addr string) (bool, error) {
		return p.store.IsSuppressed(ctx, t.OrgID, ch, addr)
	})
	if err != nil {
		return Result{}, err
	}
	msgs, err := p.buildMessages(t.Source, occ, data, cands)
	if err != nil {
		return Result{}, err
	}

	sch, err := p.schedule(ctx, t, occ, msgs)
	if err != nil {
		return Result{}, err
	}
	event := Outcome("")
	if len(subs) == 0 {
		event = OutcomeSkippedNoSubscribers
	}
	return p.finish(log, t, p.record(sch, cands, msgs, event)), nil
}

// loadSource reads the alarm or move the trigger names. A row that is not
// there is a wrong trigger, never a reason to retry: the detector committed
// it together with the job.
func (p *DeliveryPlanner) loadSource(ctx context.Context, t Trigger) (notificationevent.Occurrence, error) {
	occ, found, err := p.sources[t.Source].load(ctx, t.OrgID, t.SourceID)
	if err != nil {
		return occ, fmt.Errorf("routing: load %s %d: %w", t.Source, t.SourceID, err)
	}
	if !found {
		return occ, fmt.Errorf("%w: %s %d not found", ErrInvalidTrigger, t.Source, t.SourceID)
	}
	return occ, nil
}

// loadNames looks up the asset and location names the templates mention.
// assetFound is false when the asset has since been deleted.
func (p *DeliveryPlanner) loadNames(ctx context.Context, orgID int, occ notificationevent.Occurrence) (TemplateData, bool, error) {
	var locationIDs []int
	if occ.LocationID != nil {
		locationIDs = []int{*occ.LocationID}
	}
	assets, locations, err := p.store.LookupMoveNames(ctx, orgID, []int{occ.AssetID}, locationIDs)
	if err != nil {
		return TemplateData{}, false, fmt.Errorf("routing: look up names for asset %d: %w", occ.AssetID, err)
	}
	asset, ok := assets[occ.AssetID]
	if !ok {
		return TemplateData{}, false, nil
	}
	location := ""
	if occ.LocationID != nil {
		location = locations[*occ.LocationID]
	}
	return newTemplateData(asset, location, occ.OccurredAt), true, nil
}

// buildMessages renders every eligible candidate. Rendering happens before
// any write, so a template error leaves nothing behind. EventID is not known
// yet and is filled in inside the transaction.
func (p *DeliveryPlanner) buildMessages(source notificationevent.Source, occ notificationevent.Occurrence, data TemplateData, cands []candidate) ([]planned, error) {
	var out []planned
	for i, c := range cands {
		if c.Outcome != "" {
			continue
		}
		r, err := p.renderer.Render(source, c.Channel, data)
		if err != nil {
			return nil, fmt.Errorf("routing: render for recipient %d: %w", c.Subscriber.RecipientID, err)
		}
		out = append(out, planned{slot: i, msg: DeliveryMessage{
			Source: source, Channel: c.Channel, RecipientID: c.Subscriber.RecipientID,
			To: c.Address, Subject: r.Subject, Body: r.Body, AssetID: occ.AssetID, OccurredAt: occ.OccurredAt,
		}})
	}
	return out, nil
}

// schedule writes the event, every delivery and every send job in one
// transaction. An event that already exists means an earlier run finished, so
// nothing more is written.
func (p *DeliveryPlanner) schedule(ctx context.Context, t Trigger, occ notificationevent.Occurrence, msgs []planned) (scheduled, error) {
	var out scheduled
	err := p.store.WithOrgTx(ctx, t.OrgID, func(tx pgx.Tx) error {
		ev := notificationevent.NotificationEvent{
			OrgID: t.OrgID, Source: t.Source, AssetID: occ.AssetID, OccurredAt: occ.OccurredAt,
		}
		p.sources[t.Source].setID(&ev, t.SourceID)
		id, inserted, err := p.store.InsertNotificationEventIfAbsentTx(ctx, tx, t.OrgID, ev)
		if err != nil {
			return fmt.Errorf("routing: register event: %w", err)
		}
		if !inserted {
			out = scheduled{duplicate: true}
			return nil
		}
		cmds := make([]outbox.Command, 0, len(msgs))
		for _, m := range msgs {
			msg := m.msg
			msg.EventID = id
			payload, err := json.Marshal(msg)
			if err != nil {
				return fmt.Errorf("routing: encode message for recipient %d: %w", msg.RecipientID, err)
			}
			cmds = append(cmds, outbox.Command{
				DeliveryID: deliveryID(id, msg.RecipientID, msg.Channel), OrgID: t.OrgID, Channel: msg.Channel,
				EventID: id, RecipientID: msg.RecipientID, AssetID: msg.AssetID, Payload: payload,
			})
		}
		results, err := outbox.EnqueueBatchTx(ctx, tx, p.store, p.jobs, cmds)
		if err != nil {
			return err
		}
		out = scheduled{eventID: id, results: results}
		return nil
	})
	if err != nil {
		return scheduled{}, err
	}
	return out, nil
}

// record turns what was written into a Result. event is the event-level
// outcome to report when no duplicate overrides it. It only builds values;
// metrics and logs are emitted by finish, after the commit.
func (p *DeliveryPlanner) record(sch scheduled, cands []candidate, msgs []planned, event Outcome) Result {
	if sch.duplicate {
		return Result{Outcome: OutcomeDuplicate}
	}
	res := Result{EventID: sch.eventID, Outcome: event}
	for _, c := range cands {
		res.Recipients = append(res.Recipients, RecipientOutcome{
			RecipientID: c.Subscriber.RecipientID, Channel: c.Channel, Outcome: c.Outcome,
		})
	}
	for i, m := range msgs {
		r := &res.Recipients[m.slot]
		r.DeliveryID = sch.results[i].DeliveryID
		r.Outcome = OutcomeEnqueued
		if sch.results[i].Duplicate {
			r.Outcome = OutcomeDuplicate
		}
	}
	return res
}

// finish records metrics and logs for a committed plan. It is only called
// once the transaction (if any) has committed.
func (p *DeliveryPlanner) finish(log zerolog.Logger, t Trigger, res Result) Result {
	if res.EventID != 0 {
		log = log.With().Int("event_id", res.EventID).Logger()
	}
	counts := zerolog.Dict()
	if res.Outcome != "" {
		p.metrics.RecordOutcome(t.Source, res.Outcome)
		counts.Int(string(res.Outcome), 1)
	}
	tally := map[Outcome]int{}
	for _, r := range res.Recipients {
		switch r.Outcome {
		case OutcomeEnqueued:
			p.outboxMetrics.RecordEnqueue(outbox.EnqueueOK)
		case OutcomeDuplicate:
			p.outboxMetrics.RecordEnqueue(outbox.EnqueueDuplicate)
		}
		p.metrics.RecordOutcome(t.Source, r.Outcome)
		tally[r.Outcome]++

		ev := log.Debug()
		if r.Outcome == OutcomeSkippedOptedOut || r.Outcome == OutcomeSkippedNoAddress {
			ev = log.Warn()
		}
		ev.Int("recipient_id", r.RecipientID).Str("channel", string(r.Channel)).
			Str("delivery_id", r.DeliveryID).Str("outcome", string(r.Outcome)).Msg("notification recipient planned")
	}
	for o, n := range tally {
		counts.Int(string(o), n)
	}
	log.Info().Dict("outcomes", counts).Msg("notification planned")
	return res
}
