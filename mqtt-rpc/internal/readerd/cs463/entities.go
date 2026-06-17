package cs463

// Entity reconcile transport: list/add/mod of the reader's event-engine entities
// (CloudServer, Data Format, Triggering Logic, Resultant Action, Event) over the
// /API. Reads (list*) are the well-tested half of the API and are used for both
// existence checks and drift comparison; writes (add*/mod*) push the golden
// definitions. The golden definitions themselves live in golden.go; orchestration
// in provision.go. XML stays confined to this package.

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
)

// EntityRow is one entity's attributes keyed by the verbatim XML attribute name
// (CSL emits mixed snake/camel case, e.g. duplicateEliminationWindow, operProfile_id,
// capture_point — attrMap preserves them exactly).
type EntityRow map[string]string

// xmlRow captures every attribute of a list-response row element.
type xmlRow struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

func listParams(session, command string) url.Values {
	return url.Values{"session_id": {session}, "command": {command}}
}

func rowsToMap(rows []xmlRow, idKey string) map[string]EntityRow {
	out := make(map[string]EntityRow, len(rows))
	for _, r := range rows {
		m := EntityRow(attrMap(r.Attrs))
		if id := m[idKey]; id != "" {
			out[id] = m
		}
	}
	return out
}

// ListServer returns CloudServer entries keyed by server_id.
func (c *Client) ListServer(ctx context.Context, session string) (map[string]EntityRow, error) {
	var doc struct {
		XMLName xml.Name  `xml:"CSL"`
		Rows    []xmlRow  `xml:"ServerList>Server"`
		Error   *xmlError `xml:"Error"`
	}
	if err := c.do(ctx, listParams(session, "listServer"), &doc); err != nil {
		return nil, err
	}
	if doc.Error != nil {
		return nil, fmt.Errorf("cs463: listServer failed: %s", doc.Error.Msg)
	}
	return rowsToMap(doc.Rows, "server_id"), nil
}

// ListDataFormat returns data formats keyed by data_format_id.
func (c *Client) ListDataFormat(ctx context.Context, session string) (map[string]EntityRow, error) {
	var doc struct {
		XMLName xml.Name  `xml:"CSL"`
		Rows    []xmlRow  `xml:"DataFormatList>dataFormat"`
		Error   *xmlError `xml:"Error"`
	}
	if err := c.do(ctx, listParams(session, "listDataFormat"), &doc); err != nil {
		return nil, err
	}
	if doc.Error != nil {
		return nil, fmt.Errorf("cs463: listDataFormat failed: %s", doc.Error.Msg)
	}
	return rowsToMap(doc.Rows, "data_format_id"), nil
}

// ListTriggeringLogic returns triggering logics keyed by logic_id.
func (c *Client) ListTriggeringLogic(ctx context.Context, session string) (map[string]EntityRow, error) {
	var doc struct {
		XMLName xml.Name  `xml:"CSL"`
		Rows    []xmlRow  `xml:"TriggeringLogic>logic"`
		Error   *xmlError `xml:"Error"`
	}
	if err := c.do(ctx, listParams(session, "listTriggeringLogic"), &doc); err != nil {
		return nil, err
	}
	if doc.Error != nil {
		return nil, fmt.Errorf("cs463: listTriggeringLogic failed: %s", doc.Error.Msg)
	}
	return rowsToMap(doc.Rows, "logic_id"), nil
}

// ListResultantAction returns resultant actions keyed by action_id.
func (c *Client) ListResultantAction(ctx context.Context, session string) (map[string]EntityRow, error) {
	var doc struct {
		XMLName xml.Name  `xml:"CSL"`
		Rows    []xmlRow  `xml:"ResultantActionList>resultantaction"`
		Error   *xmlError `xml:"Error"`
	}
	if err := c.do(ctx, listParams(session, "listResultantAction"), &doc); err != nil {
		return nil, err
	}
	if doc.Error != nil {
		return nil, fmt.Errorf("cs463: listResultantAction failed: %s", doc.Error.Msg)
	}
	return rowsToMap(doc.Rows, "action_id"), nil
}

// ListEvent returns event definitions keyed by event_id.
func (c *Client) ListEvent(ctx context.Context, session string) (map[string]EntityRow, error) {
	var doc struct {
		XMLName xml.Name  `xml:"CSL"`
		Rows    []xmlRow  `xml:"EventList>event"`
		Error   *xmlError `xml:"Error"`
	}
	if err := c.do(ctx, listParams(session, "listEvent"), &doc); err != nil {
		return nil, err
	}
	if doc.Error != nil {
		return nil, fmt.Errorf("cs463: listEvent failed: %s", doc.Error.Msg)
	}
	return rowsToMap(doc.Rows, "event_id"), nil
}

// ListProfileIDs returns the set of operation-profile ids present on the reader
// (getOperProfile returns all profiles; we only need their ids for existence checks).
func (c *Client) ListProfileIDs(ctx context.Context, session string) (map[string]bool, error) {
	params := url.Values{"session_id": {session}, "command": {"getOperProfile"}, "profile_id": {"_"}}
	var list xmlProfileList
	if err := c.do(ctx, params, &list); err != nil {
		return nil, err
	}
	if list.Error != nil {
		return nil, fmt.Errorf("cs463: getOperProfile failed: %s", list.Error.Msg)
	}
	out := make(map[string]bool, len(list.Profiles))
	for _, p := range list.Profiles {
		if id := attrMap(p.Attrs)["profile_id"]; id != "" {
			out[id] = true
		}
	}
	return out, nil
}

// --- writes (add/mod) -----------------------------------------------------

// writeEntity issues an add*/mod* command with the given entity params (session_id
// and command are injected) and checks the reader acked OK.
func (c *Client) writeEntity(ctx context.Context, session, command string, p url.Values) error {
	params := url.Values{"session_id": {session}, "command": {command}}
	for k, vs := range p {
		for _, v := range vs {
			params.Add(k, v)
		}
	}
	var ack xmlAck
	if err := c.do(ctx, params, &ack); err != nil {
		return err
	}
	if ack.Error != nil {
		return fmt.Errorf("cs463: %s failed: %s", command, ack.Error.Msg)
	}
	if !strings.HasPrefix(ack.Ack, "OK") {
		return fmt.Errorf("cs463: %s not acked: %q", command, ack.Ack)
	}
	return nil
}

func (c *Client) AddDataFormat(ctx context.Context, session string, p url.Values) error {
	return c.writeEntity(ctx, session, "addDataFormat", p)
}
func (c *Client) ModDataFormat(ctx context.Context, session string, p url.Values) error {
	return c.writeEntity(ctx, session, "modDataFormat", p)
}
func (c *Client) AddTriggeringLogic(ctx context.Context, session string, p url.Values) error {
	return c.writeEntity(ctx, session, "addTriggeringLogic", p)
}
func (c *Client) ModTriggeringLogic(ctx context.Context, session string, p url.Values) error {
	return c.writeEntity(ctx, session, "modTriggeringLogic", p)
}
func (c *Client) AddResultantAction(ctx context.Context, session string, p url.Values) error {
	return c.writeEntity(ctx, session, "addResultantAction", p)
}
func (c *Client) ModResultantAction(ctx context.Context, session string, p url.Values) error {
	return c.writeEntity(ctx, session, "modResultantAction", p)
}
func (c *Client) AddEvent(ctx context.Context, session string, p url.Values) error {
	return c.writeEntity(ctx, session, "addEvent", p)
}
func (c *Client) ModEvent(ctx context.Context, session string, p url.Values) error {
	return c.writeEntity(ctx, session, "modEvent", p)
}
