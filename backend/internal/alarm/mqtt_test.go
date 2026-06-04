package alarm

import (
	"context"
	"testing"
)

func TestMQTTPublisher_PublishesShellyCommand(t *testing.T) {
	var gotTopic string
	var gotPayload string
	p := &MQTTPublisher{publish: func(topic string, payload []byte) error {
		gotTopic, gotPayload = topic, string(payload)
		return nil
	}}

	if err := p.Publish(context.Background(), "trakrf.id/dock-strobe", 0, true); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if gotTopic != "trakrf.id/dock-strobe/command/switch:0" {
		t.Errorf("topic = %q, want trakrf.id/dock-strobe/command/switch:0", gotTopic)
	}
	if gotPayload != "on" {
		t.Errorf("payload = %q, want on", gotPayload)
	}
}

func TestMQTTPublisher_OffAndSwitchID(t *testing.T) {
	var gotTopic, gotPayload string
	p := &MQTTPublisher{publish: func(topic string, payload []byte) error {
		gotTopic, gotPayload = topic, string(payload)
		return nil
	}}

	if err := p.Publish(context.Background(), "trakrf.id/dock-strobe", 2, false); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if gotTopic != "trakrf.id/dock-strobe/command/switch:2" {
		t.Errorf("topic = %q, want .../switch:2", gotTopic)
	}
	if gotPayload != "off" {
		t.Errorf("payload = %q, want off", gotPayload)
	}
}
