package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// cmdPower handles `reboot` and `shutdown`: a clean reboot or power-off of the
// reader host. Both take the reader offline, so both need -yes.
func cmdPower(action string, args []string) error {
	fs, base, ca := rpcFlags(action)
	yes := fs.Bool("yes", false, "confirm: this takes the reader offline")
	by := fs.String("by", defaultRequestedBy(), "who is asking, for the reader's log")
	_ = fs.Parse(args)
	if *base == "" {
		return errors.New("-base is required")
	}
	method, params, err := powerRequest(action, *yes, *by)
	if err != nil {
		return err
	}
	return rpc(*base, *ca, method, params)
}

// powerRequest builds the Reader.Reboot or Reader.Shutdown request, refusing
// without confirmation.
func powerRequest(action string, yes bool, by string) (string, json.RawMessage, error) {
	var method, cost string
	switch action {
	case "reboot":
		method = readerrpc.MethodReboot
		cost = "the reader stops reading and is off the broker for several minutes (2-7 after boot)"
	case "shutdown":
		method = readerrpc.MethodShutdown
		cost = "the reader powers off and comes back only when someone on site power-cycles it"
	default:
		return "", nil, fmt.Errorf("unknown power action %q", action)
	}
	if !yes {
		return "", nil, fmt.Errorf("%s: %s; pass -yes to go ahead", action, cost)
	}
	params, err := json.Marshal(readerrpc.PowerParams{RequestedBy: by})
	return method, params, err
}

// defaultRequestedBy is $USER@hostname, or whichever half is known, or "" when
// neither is: never a bare "user@" or "@host".
func defaultRequestedBy() string {
	user := os.Getenv("USER")
	host, _ := os.Hostname()
	switch {
	case user != "" && host != "":
		return user + "@" + host
	case user != "":
		return user
	default:
		return host
	}
}
