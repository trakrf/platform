// Command mqtt-rpcctl is the operator and CI tool for mqtt-rpcd releases and RPC.
// It runs on an operator machine or in CI, never on a reader (unlike mqtt-rpcd,
// it takes flags).
//
//	mqtt-rpcctl keygen -dir DIR -id ID
//	mqtt-rpcctl sign   -version V -binary PATH -url URL [-key FILE] [-out FILE]
//	mqtt-rpcctl call   -base TOPIC -method M [-params JSON] [-ca FILE]
//	mqtt-rpcctl update -base TOPIC -manifest FILE|URL [-ca FILE]
//
// sign reads the key from -key or, if absent, from $MQTT_RPCD_SIGNING_KEY.
// call/update read the broker URL (with credentials) from $MQTT_URL so they never
// appear on a command line. -base is the reader's base topic, e.g.
// trakrf.id/cs463-212.
package main

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		fs := flag.NewFlagSet("keygen", flag.ExitOnError)
		dir := fs.String("dir", "", "directory for the key pair")
		id := fs.String("id", "", "key id, e.g. ci-2026-11")
		_ = fs.Parse(os.Args[2:])
		if *dir == "" || *id == "" {
			usage()
		}
		err = keygen(*dir, *id)
	case "sign":
		err = cmdSign(os.Args[2:])
	case "call":
		err = cmdCall(os.Args[2:])
	case "update":
		err = cmdUpdate(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mqtt-rpcctl:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: mqtt-rpcctl keygen|sign|call|update [flags]  (see -h on each)")
	os.Exit(2)
}

func cmdSign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	version := fs.String("version", "", "release version (must equal the binary's -X main.version)")
	bin := fs.String("binary", "", "path to the linux/armv7 binary")
	url := fs.String("url", "", "https URL the binary will be served from")
	keyFile := fs.String("key", "", "base64 seed file (default: $MQTT_RPCD_SIGNING_KEY)")
	out := fs.String("out", "", "write the manifest here (default stdout)")
	unchecked := fs.Bool("unchecked-version", false, "skip the check that the binary was built as -version (bench fixtures only)")
	_ = fs.Parse(args)

	seed := os.Getenv("MQTT_RPCD_SIGNING_KEY")
	if *keyFile != "" {
		b, err := os.ReadFile(*keyFile)
		if err != nil {
			return err
		}
		seed = string(b)
	}
	if seed == "" {
		return errors.New("no signing key: pass -key or set MQTT_RPCD_SIGNING_KEY")
	}
	priv, err := parseSeed(seed)
	if err != nil {
		return err
	}
	m, err := signRelease(priv, *version, *bin, *url, !*unchecked)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	b = append(b, '\n')
	if *out == "" {
		_, err = os.Stdout.Write(b)
		return err
	}
	return os.WriteFile(*out, b, 0o644)
}

func rpcFlags(name string) (*flag.FlagSet, *string, *string) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	base := fs.String("base", "", "reader base topic, e.g. trakrf.id/cs463-212")
	ca := fs.String("ca", "", "broker CA bundle (default: system roots)")
	return fs, base, ca
}

func cmdCall(args []string) error {
	fs, base, ca := rpcFlags("call")
	method := fs.String("method", "", "RPC method, e.g. Reader.GetCapabilities")
	params := fs.String("params", "", "JSON params")
	_ = fs.Parse(args)
	if *base == "" || *method == "" {
		return errors.New("-base and -method are required")
	}
	var raw json.RawMessage
	if *params != "" {
		raw = json.RawMessage(*params)
	}
	return rpc(*base, *ca, *method, raw)
}

func cmdUpdate(args []string) error {
	fs, base, ca := rpcFlags("update")
	src := fs.String("manifest", "", "manifest.json path or https URL")
	_ = fs.Parse(args)
	if *base == "" || *src == "" {
		return errors.New("-base and -manifest are required")
	}
	m, err := loadManifest(*src)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(m)
	return rpc(*base, *ca, readerrpc.MethodDaemonUpdate, raw)
}

// rpc sends one request to <base>/rpc and prints the reply frame.
func rpc(base, caFile, method string, params json.RawMessage) error {
	brokerURL := os.Getenv("MQTT_URL")
	if brokerURL == "" {
		return errors.New("set MQTT_URL (e.g. mqtts://user:pass@mqtt.preview.gke.trakrf.id:8883)")
	}
	nonce := make([]byte, 6)
	_, _ = rand.Read(nonce)
	id := hex.EncodeToString(nonce)
	replyTopic := "trakrf-cli/" + id + "/reply"

	opts := mqtt.NewClientOptions().AddBroker(brokerURL).SetClientID("mqtt-rpcctl-" + id).SetCleanSession(true)
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return fmt.Errorf("no certificates in %s", caFile)
		}
		opts.SetTLSConfig(&tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12})
	}
	c := mqtt.NewClient(opts)
	if tok := c.Connect(); !tok.WaitTimeout(20*time.Second) || tok.Error() != nil {
		return fmt.Errorf("connect: %v", tok.Error())
	}
	defer c.Disconnect(250)

	got := make(chan []byte, 1)
	if tok := c.Subscribe(replyTopic, 1, func(_ mqtt.Client, m mqtt.Message) { got <- m.Payload() }); !tok.WaitTimeout(10*time.Second) || tok.Error() != nil {
		return fmt.Errorf("subscribe: %v", tok.Error())
	}
	req, _ := json.Marshal(readerrpc.Request{ID: 1, Src: replyTopic, Method: method, Params: params})
	if tok := c.Publish(readerrpc.RPCTopic(base), 1, false, req); !tok.WaitTimeout(10*time.Second) || tok.Error() != nil {
		return fmt.Errorf("publish: %v", tok.Error())
	}
	select {
	case b := <-got:
		fmt.Println(string(b))
		var resp readerrpc.Response
		if json.Unmarshal(b, &resp) == nil && resp.Error != nil {
			return fmt.Errorf("reader error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil
	case <-time.After(15 * time.Second):
		return errors.New("no reply within 15s (daemon offline?)")
	}
}
