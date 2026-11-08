package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// cmdStatus prints a reader's retained <base>/status document: presence, daemon
// version and reader-API health. A wedged vendor app shows up here, not in
// systemd. With -watch it keeps printing one summary line per change.
func cmdStatus(args []string) error {
	fs, base, ca := rpcFlags("status")
	watch := fs.Bool("watch", false, "keep printing a summary line per status change until interrupted")
	_ = fs.Parse(args)
	if *base == "" {
		return errors.New("-base is required")
	}
	c, err := connect(*ca)
	if err != nil {
		return err
	}
	defer c.Disconnect(250)

	got := make(chan []byte, 16)
	topic := readerrpc.StatusTopic(*base)
	if tok := c.Subscribe(topic, 1, func(_ mqtt.Client, m mqtt.Message) { got <- m.Payload() }); !tok.WaitTimeout(10*time.Second) || tok.Error() != nil {
		return fmt.Errorf("subscribe: %v", tok.Error())
	}

	if !*watch {
		select {
		case b := <-got:
			fmt.Println(string(b))
			return nil
		case <-time.After(10 * time.Second):
			return fmt.Errorf("no retained status on %s within 10s", topic)
		}
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	for {
		select {
		case b := <-got:
			fmt.Printf("%s %s\n", time.Now().UTC().Format("15:04:05"), summarizeStatus(b))
		case <-stop:
			return nil
		}
	}
}

// summarizeStatus renders a status document as one line, loudest facts first.
func summarizeStatus(b []byte) string {
	var doc readerrpc.StatusDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return "unparseable: " + string(b)
	}
	parts := []string{"offline"}
	if doc.Online {
		parts[0] = "online"
	} else if doc.Reason != "" {
		parts[0] = "offline (" + doc.Reason + ")"
	}
	if doc.Version != "" {
		parts = append(parts, doc.Version)
	}
	if h := doc.Health; h != nil {
		parts = append(parts, "api="+h.API)
		if h.Wedged {
			parts = append(parts, "WEDGED")
		}
		if h.Recovery != "" {
			parts = append(parts, "recovery="+h.Recovery)
		}
		if h.AutoReboots > 0 {
			parts = append(parts, fmt.Sprintf("auto_reboots=%d", h.AutoReboots))
		}
		if h.AutoRebootsExhausted {
			parts = append(parts, "EXHAUSTED")
		}
		parts = append(parts, "golden="+h.GoldenConfig)
		if h.CloudServerFault != "" {
			parts = append(parts, fmt.Sprintf("cloudserver_fault=%q", h.CloudServerFault))
		}
		if h.Detail != "" && h.API != readerrpc.APIHealthy {
			parts = append(parts, fmt.Sprintf("detail=%q", h.Detail))
		}
	}
	return strings.Join(parts, " ")
}
