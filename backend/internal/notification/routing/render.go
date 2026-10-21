package routing

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/storage"
)

//go:embed templates.yaml
var templatesYAML []byte

const genericKey = "generic"

// TemplateData is everything a message template may mention. Values are
// inserted as plain text; they are never parsed as templates themselves.
type TemplateData struct {
	AssetName, AssetKey, LocationName, OccurredAt string
}

// newTemplateData builds the template inputs. An empty locationName reads as
// an unknown location; the time is shown in UTC.
func newTemplateData(asset storage.AssetIdentity, locationName string, at time.Time) TemplateData {
	if locationName == "" {
		locationName = "an unknown location"
	}
	return TemplateData{
		AssetName:    asset.Name,
		AssetKey:     asset.ExternalKey,
		LocationName: locationName,
		OccurredAt:   at.UTC().Format("2006-01-02 15:04"),
	}
}

// Rendered is a finished message. Subject is empty for channels without one.
type Rendered struct{ Subject, Body string }

// Renderer turns a notification into message text.
type Renderer interface {
	Render(source notificationevent.Source, ch notificationdelivery.Channel, d TemplateData) (Rendered, error)
}

type compiled struct {
	subject *template.Template // nil for sms
	body    *template.Template
}

// TemplateRenderer renders from templates parsed once at construction.
type TemplateRenderer struct {
	byKey map[string]compiled
}

type rawTemplate struct {
	Subject string `yaml:"subject"`
	Body    string `yaml:"body"`
}

// NewTemplateRenderer loads the embedded templates, failing if any is broken.
func NewTemplateRenderer() (*TemplateRenderer, error) {
	return parseTemplates(templatesYAML)
}

func templateKey(source, channel string) string { return source + "." + channel }

// parseTemplates validates and compiles a template file. It is the startup
// check: every entry must parse and render against sample data.
func parseTemplates(data []byte) (*TemplateRenderer, error) {
	var raw map[string]map[string]rawTemplate
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	sample := TemplateData{"Forklift 7", "ASSET-0007", "Dock 3", "2026-10-20 14:05"}
	r := &TemplateRenderer{byKey: map[string]compiled{}}
	for source, channels := range raw {
		switch source {
		case genericKey, string(notificationevent.SourceAlarm), string(notificationevent.SourceLocationChange):
		default:
			return nil, fmt.Errorf("templates: unknown source %q", source)
		}
		for channel, t := range channels {
			key := templateKey(source, channel)
			var c compiled
			switch notificationdelivery.Channel(channel) {
			case notificationdelivery.ChannelEmail:
				if t.Subject == "" || t.Body == "" {
					return nil, fmt.Errorf("templates: %s needs subject and body", key)
				}
				s, err := compile(key+".subject", t.Subject)
				if err != nil {
					return nil, err
				}
				c.subject = s
			case notificationdelivery.ChannelSMS:
				if t.Body == "" {
					return nil, fmt.Errorf("templates: %s needs body", key)
				}
			default:
				return nil, fmt.Errorf("templates: unknown channel %q in %s", channel, source)
			}
			b, err := compile(key+".body", t.Body)
			if err != nil {
				return nil, err
			}
			c.body = b
			r.byKey[key] = c
		}
	}

	for _, ch := range []notificationdelivery.Channel{notificationdelivery.ChannelEmail, notificationdelivery.ChannelSMS} {
		if _, ok := r.byKey[templateKey(genericKey, string(ch))]; !ok {
			return nil, fmt.Errorf("templates: %s is required", templateKey(genericKey, string(ch)))
		}
	}
	for key, c := range r.byKey {
		if _, err := c.execute(sample); err != nil {
			return nil, fmt.Errorf("templates: %s: %w", key, err)
		}
	}
	return r, nil
}

func compile(name, text string) (*template.Template, error) {
	t, err := template.New(name).Option("missingkey=error").Parse(text)
	if err != nil {
		return nil, fmt.Errorf("templates: %s: %w", name, err)
	}
	return t, nil
}

func (c compiled) execute(d TemplateData) (Rendered, error) {
	var out Rendered
	var buf bytes.Buffer
	if c.subject != nil {
		if err := c.subject.Execute(&buf, d); err != nil {
			return Rendered{}, err
		}
		out.Subject = buf.String()
		buf.Reset()
	}
	if err := c.body.Execute(&buf, d); err != nil {
		return Rendered{}, err
	}
	out.Body = buf.String()
	return out, nil
}

// Render uses the template for the source and channel, else the generic one.
func (r *TemplateRenderer) Render(source notificationevent.Source, ch notificationdelivery.Channel, d TemplateData) (Rendered, error) {
	c, ok := r.byKey[templateKey(string(source), string(ch))]
	if !ok {
		c, ok = r.byKey[templateKey(genericKey, string(ch))]
	}
	if !ok {
		return Rendered{}, fmt.Errorf("no template for %s.%s", source, ch)
	}
	out, err := c.execute(d)
	if err != nil {
		return Rendered{}, fmt.Errorf("render %s.%s: %w", source, ch, err)
	}
	return out, nil
}
