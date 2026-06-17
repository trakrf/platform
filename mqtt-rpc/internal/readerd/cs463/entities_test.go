package cs463

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// entFake serves a canned XML body per /API command and records each request's
// query, so entity read/write tests can assert both parsing and the params sent.
type entFake struct {
	srv       *httptest.Server
	bodies    map[string]string
	lastQuery url.Values
}

func newEntFake(bodies map[string]string) *entFake {
	f := &entFake{bodies: bodies}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.lastQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/xml")
		body := f.bodies[r.URL.Query().Get("command")]
		if body == "" {
			body = ackOK // reuse the OK ack defined in csl_test.go
		}
		_, _ = w.Write([]byte(body))
	}))
	return f
}

func (f *entFake) client() *Client { return New(f.srv.URL, "root", "pw", 0) }
func (f *entFake) close()          { f.srv.Close() }

const listEventBody = `<?xml version="1.0" ?><CSL><Command>listEvent</Command><EventMode mode="0"/>` +
	`<EventList><event antennaDifferentiation="true" desc="" duplicateEliminationWindow="500" ` +
	`enable="true" event_id="TrakRF mqtt-rpc Event" exclusivity="Non-exclusive" ` +
	`inventoryDisablingAction="NONE" inventoryDisablingTrigger="Never Stop" ` +
	`inventoryEnablingAction="NONE" inventoryEnablingTrigger="Always On" ` +
	`operProfile_id="TrakRF mqtt-rpc Profile" resultant_action="TrakRF mqtt-rpc Action" ` +
	`triggering_logic="TrakRF mqtt-rpc Trigger"/></EventList></CSL>`

const listTriggerBody = `<?xml version="1.0" ?><CSL><Command>listTriggeringLogic</Command><TriggeringLogic>` +
	`<logic capture_point="12" desc="" logic="" logic_id="TrakRF mqtt-rpc Trigger" ` +
	`mode="Read Any Tags (any ID, 1 trigger per tag)" referenceTagId="" state_mode=""/>` +
	`</TriggeringLogic></CSL>`

const listActionBody = `<?xml version="1.0" ?><CSL><Command>listResultantAction</Command><ResultantActionList>` +
	`<resultantaction action="" action_id="TrakRF mqtt-rpc Action" action_mode="Low Latency Alert to Server" ` +
	`data_format_id="TrakRF mqtt-rpc Data Format" server_id="TrakRF mqtt-rpc MQTT Server" transport="MQTT"/>` +
	`</ResultantActionList></CSL>`

const listDataFormatBody = `<?xml version="1.0" ?><CSL><Command>listDataFormat</Command><DataFormatList>` +
	`<dataFormat data_format_id="TrakRF mqtt-rpc Data Format" desc="" format="JSON" ` +
	`field1="SequenceNumber" field2="NumberOfTags" field3="TagDataList" ` +
	`label1="sequenceNumber" label2="numberOfTags" label3="tags" ` +
	`tagDataField1="EPC" tagDataField2="TimeStampOfRead" tagDataField3="AntennaPort_Number" tagDataField4="RSSI_Number" ` +
	`tagDataLabel1="epc" tagDataLabel2="timeStampOfRead" tagDataLabel3="antennaPort" tagDataLabel4="rssi"/>` +
	`</DataFormatList></CSL>`

const listServerBody = `<?xml version="1.0" ?><CSL><Command>listServer</Command><ServerList>` +
	`<Server desc="" server_id="TrakRF mqtt-rpc MQTT Server" server_ip="mqtt.preview.gke.trakrf.id" server_port="8883" type="MQTT"/>` +
	`</ServerList></CSL>`

func TestListEventParsesAttrs(t *testing.T) {
	f := newEntFake(map[string]string{"listEvent": listEventBody})
	defer f.close()
	rows, err := f.client().ListEvent(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := rows[NameEvent]
	if !ok {
		t.Fatalf("event %q not found in %v", NameEvent, rows)
	}
	if ev["duplicateEliminationWindow"] != "500" || ev["antennaDifferentiation"] != "true" ||
		ev["operProfile_id"] != NameProfile || ev["triggering_logic"] != NameTrigger ||
		ev["resultant_action"] != NameAction {
		t.Fatalf("bad event attrs: %v", ev)
	}
}

func TestListTriggeringLogicParsesAttrs(t *testing.T) {
	f := newEntFake(map[string]string{"listTriggeringLogic": listTriggerBody})
	defer f.close()
	rows, err := f.client().ListTriggeringLogic(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	if rows[NameTrigger]["capture_point"] != "12" || rows[NameTrigger]["mode"] != "Read Any Tags (any ID, 1 trigger per tag)" {
		t.Fatalf("bad trigger row: %v", rows)
	}
}

func TestListResultantActionParsesAttrs(t *testing.T) {
	f := newEntFake(map[string]string{"listResultantAction": listActionBody})
	defer f.close()
	rows, err := f.client().ListResultantAction(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	if rows[NameAction]["server_id"] != NameMQTTServer || rows[NameAction]["data_format_id"] != NameDataFormat ||
		rows[NameAction]["transport"] != "MQTT" {
		t.Fatalf("bad action row: %v", rows)
	}
}

func TestListDataFormatParsesAttrs(t *testing.T) {
	f := newEntFake(map[string]string{"listDataFormat": listDataFormatBody})
	defer f.close()
	rows, err := f.client().ListDataFormat(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	df := rows[NameDataFormat]
	if df["format"] != "JSON" || df["tagDataField4"] != "RSSI_Number" || df["tagDataField3"] != "AntennaPort_Number" {
		t.Fatalf("bad data format row: %v", df)
	}
}

func TestListServerParsesAttrs(t *testing.T) {
	f := newEntFake(map[string]string{"listServer": listServerBody})
	defer f.close()
	rows, err := f.client().ListServer(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rows[NameMQTTServer]; !ok {
		t.Fatalf("server %q not found in %v", NameMQTTServer, rows)
	}
}

func TestAddEventSendsParamsAndChecksAck(t *testing.T) {
	f := newEntFake(map[string]string{
		"addEvent": `<?xml version="1.0" ?><CSL><Command>addEvent</Command><Ack>OK:</Ack></CSL>`,
	})
	defer f.close()
	p := url.Values{"event_id": {NameEvent}, "duplicateEliminationWindow": {"500"}}
	if err := f.client().AddEvent(context.Background(), "sid", p); err != nil {
		t.Fatal(err)
	}
	if f.lastQuery.Get("command") != "addEvent" || f.lastQuery.Get("duplicateEliminationWindow") != "500" ||
		f.lastQuery.Get("session_id") != "sid" || f.lastQuery.Get("event_id") != NameEvent {
		t.Fatalf("bad query: %v", f.lastQuery)
	}
}

func TestWriteEntityNonOKAckErrors(t *testing.T) {
	f := newEntFake(map[string]string{
		"addEvent": `<?xml version="1.0" ?><CSL><Command>addEvent</Command><Ack>Error: bad request</Ack></CSL>`,
	})
	defer f.close()
	err := f.client().AddEvent(context.Background(), "sid", url.Values{})
	if err == nil || !strings.Contains(err.Error(), "not acked") {
		t.Fatalf("expected not-acked error, got %v", err)
	}
}

func TestWriteEntityErrorElementErrors(t *testing.T) {
	f := newEntFake(map[string]string{
		"modEvent": `<?xml version="1.0" ?><CSL><Command>modEvent</Command><Error code="-1" msg="no such event"/></CSL>`,
	})
	defer f.close()
	if err := f.client().ModEvent(context.Background(), "sid", url.Values{}); err == nil {
		t.Fatal("expected error from <Error> element")
	}
}
