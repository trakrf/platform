package routing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/storage"
)

var sampleData = TemplateData{"Forklift 7", "ASSET-0007", "Dock 3", "2026-10-20 14:05"}

const genericOnly = `
generic:
  email:
    subject: "G {{.AssetName}}"
    body: "gb {{.AssetKey}}"
  sms:
    body: "gs {{.AssetKey}}"
`

func TestRender_PerSource(t *testing.T) {
	r, err := NewTemplateRenderer()
	require.NoError(t, err)

	got, err := r.Render(notificationevent.SourceAlarm, notificationdelivery.ChannelSMS, sampleData)
	require.NoError(t, err)
	assert.Equal(t, "TrakRF: Forklift 7 triggered an alarm at Dock 3 at 2026-10-20 14:05 UTC.", got.Body)

	got, err = r.Render(notificationevent.SourceLocationChange, notificationdelivery.ChannelEmail, sampleData)
	require.NoError(t, err)
	assert.Equal(t, "Location update: Forklift 7", got.Subject)
}

func TestRender_GenericFallback(t *testing.T) {
	r, err := parseTemplates([]byte(genericOnly))
	require.NoError(t, err)

	got, err := r.Render(notificationevent.SourceAlarm, notificationdelivery.ChannelEmail, sampleData)
	require.NoError(t, err)
	assert.Equal(t, "G Forklift 7", got.Subject)
}

func TestNewTemplateData(t *testing.T) {
	asset := storage.AssetIdentity{ID: 1, ExternalKey: "K1", Name: "N1"}

	d := newTemplateData(asset, "", time.Date(2026, 10, 20, 16, 5, 0, 0, time.FixedZone("EAT", 3*3600)))
	assert.Equal(t, "an unknown location", d.LocationName)
	assert.Equal(t, "2026-10-20 13:05", d.OccurredAt)
	assert.Equal(t, "N1", d.AssetName)
	assert.Equal(t, "K1", d.AssetKey)
}

func TestRender_AssetNameIsNotATemplate(t *testing.T) {
	r, err := NewTemplateRenderer()
	require.NoError(t, err)

	d := sampleData
	d.AssetName = "{{.AssetKey}}"
	got, err := r.Render(notificationevent.SourceAlarm, notificationdelivery.ChannelSMS, d)
	require.NoError(t, err)
	assert.Contains(t, got.Body, "{{.AssetKey}}")
}

func TestParseTemplates_Rejects(t *testing.T) {
	cases := map[string]string{
		"missing generic.sms": `
generic:
  email:
    subject: "s"
    body: "b"
`,
		"email without subject": `
generic:
  email:
    body: "b"
  sms:
    body: "b"
`,
		"unknown top-level key": genericOnly + `
geofence:
  sms:
    body: "b"
`,
		"unknown channel": genericOnly + `
alarm:
  fax:
    body: "b"
`,
		"unparseable template": `
generic:
  email:
    subject: "s"
    body: "{{.AssetName"
  sms:
    body: "b"
`,
		"unknown field": `
generic:
  email:
    subject: "s"
    body: "{{.Nope}}"
  sms:
    body: "b"
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseTemplates([]byte(src))
			assert.Error(t, err)
		})
	}
}

func TestNewTemplateRenderer_EmbeddedIsValid(t *testing.T) {
	_, err := NewTemplateRenderer()
	assert.NoError(t, err)
}
