package alarm

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	clientmodel "github.com/prometheus/client_model/go"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

type failingHTTP struct{ err error }

func (h failingHTTP) Set(context.Context, string, int, bool, int) error { return h.err }

type failingMQTT struct{ err error }

func (m failingMQTT) Publish(context.Context, string, int, bool, int) error { return m.err }

type failingGPO struct{ err error }

func (g failingGPO) GpoSet(context.Context, string, int, bool, int) error { return g.err }

// timeoutErr mimics a net/http client timeout (url.Error wrapping a net.Error).
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestDeviceCallMetricsRecordsKindAndOutcome(t *testing.T) {
	topic := "trakrf.id/shelly-1"
	cases := []struct {
		name    string
		dev     outputdevice.OutputDevice
		err     error
		kind    string
		outcome string
	}{
		{"shelly http success", outputdevice.OutputDevice{ID: 1, Transport: outputdevice.TransportHTTP, BaseURL: "http://shelly"}, nil, kindShellyHTTP, outcomeSuccess},
		{"shelly http error", outputdevice.OutputDevice{ID: 1, Transport: outputdevice.TransportHTTP, BaseURL: "http://shelly"}, errors.New("status 500"), kindShellyHTTP, outcomeError},
		{"shelly http client timeout", outputdevice.OutputDevice{ID: 1, Transport: outputdevice.TransportHTTP, BaseURL: "http://shelly"}, &url.Error{Op: "Post", URL: "http://shelly/rpc", Err: timeoutErr{}}, kindShellyHTTP, outcomeTimeout},
		{"shelly mqtt publish timeout", outputdevice.OutputDevice{ID: 2, Transport: outputdevice.TransportMQTT, CommandTopic: &topic}, fmt.Errorf("publish: %w", ErrPublishTimeout), kindShellyMQTT, outcomeTimeout},
		{"gpo context deadline", outputdevice.OutputDevice{ID: 3, Type: outputdevice.TypeCSLGPO, Transport: outputdevice.TransportMQTT, ReaderBaseTopic: "trakrf.id/reader-1", SwitchID: 1}, fmt.Errorf("notify: %w", context.DeadlineExceeded), kindCSLGPO, outcomeTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			d := NewDispatcher(failingHTTP{tc.err}, failingMQTT{tc.err}, failingGPO{tc.err}).WithMetrics(NewDeviceCallMetrics(registry))

			if err := d.Set(context.Background(), tc.dev, true, 0); !errors.Is(err, tc.err) {
				t.Fatalf("Set error = %v, want %v", err, tc.err)
			}
			assertHistogramCount(t, registry, map[string]string{"kind": tc.kind, "outcome": tc.outcome}, 1)
		})
	}
}

// A device rejected by validation never reached a transport, so it is not a
// device call and must not skew the latency distribution.
func TestDeviceCallMetricsSkipsValidationRejects(t *testing.T) {
	registry := prometheus.NewRegistry()
	d := NewDispatcher(failingHTTP{}, nil, nil).WithMetrics(NewDeviceCallMetrics(registry))

	dev := outputdevice.OutputDevice{ID: 4, Transport: outputdevice.TransportMQTT}
	if err := d.Set(context.Background(), dev, true, 0); err == nil {
		t.Fatal("Set succeeded for an mqtt device with no command_topic")
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == deviceCallMetricName && len(family.GetMetric()) != 0 {
			t.Fatalf("%s recorded a validation reject", deviceCallMetricName)
		}
	}
}

// Without metrics the dispatcher behaves exactly as before.
func TestDispatcherWithoutMetricsStillDispatches(t *testing.T) {
	http := &recordingHTTP{}
	d := NewDispatcher(http, nil, nil)
	if err := d.Set(context.Background(), outputdevice.OutputDevice{ID: 5, BaseURL: "http://shelly"}, true, 0); err != nil {
		t.Fatal(err)
	}
	if !http.called {
		t.Fatal("http setter was not called")
	}
}

func TestDeviceCallMetricsRegistrationCanBeRepeated(t *testing.T) {
	registry := prometheus.NewRegistry()
	first := NewDeviceCallMetrics(registry)
	second := NewDeviceCallMetrics(registry)
	if first.duration != second.duration {
		t.Fatal("second registration did not reuse the existing collector")
	}
}

func assertHistogramCount(t *testing.T, registry *prometheus.Registry, labels map[string]string, want uint64) {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != deviceCallMetricName {
			continue
		}
		for _, metric := range family.GetMetric() {
			if labelsEqual(metric.GetLabel(), labels) {
				if got := metric.GetHistogram().GetSampleCount(); got != want {
					t.Fatalf("%s%v sample count = %d, want %d", deviceCallMetricName, labels, got, want)
				}
				return
			}
		}
	}
	t.Fatalf("%s with labels %v was not found", deviceCallMetricName, labels)
}

func labelsEqual(actual []*clientmodel.LabelPair, expected map[string]string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for _, label := range actual {
		if expected[label.GetName()] != label.GetValue() {
			return false
		}
	}
	return true
}
