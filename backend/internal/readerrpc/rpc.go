// Package readerrpc defines the durable MQTT JSON-RPC control contract spoken
// between the cloud platform and an on-reader daemon (TRA-993).
//
// The contract is vendor-neutral: it carries no reader/model-specific concepts.
// Frames follow the Shelly Gen4 JSON-RPC style, where a request names the topic
// to reply to in its "src" field and the response echoes it back in "dst".
//
// This package is stdlib-only by design so both sides can depend on it without
// pulling in platform-specific dependencies.
package readerrpc

import "encoding/json"

// ContractVersion is the version of this RPC contract. Bump on a breaking
// change to frames, methods, or config shapes.
const ContractVersion = 1

// Method names. Reader.* and the Get/Set config/status/capabilities methods are
// implemented today; the remaining methods are reserved for future use.
const (
	MethodGetCapabilities = "Reader.GetCapabilities"
	MethodGetConfig       = "Reader.GetConfig"
	MethodSetConfig       = "Reader.SetConfig"
	MethodGetStatus       = "Reader.GetStatus"

	// Reserved for future use.
	MethodScanStart = "Scan.Start"
	MethodScanStop  = "Scan.Stop"
	MethodGpoSet    = "Gpo.Set"
	MethodReboot    = "Reader.Reboot"
)

// Standard JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603
)

// Request is an inbound JSON-RPC request frame. Src names the topic the caller
// expects the response to be published to (Shelly-style correlation).
type Request struct {
	ID     int             `json:"id"`
	Src    string          `json:"src"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// RPCError is the error object of an error response frame.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Response is an outbound JSON-RPC response frame. Exactly one of Result or
// Error is set. Dst echoes the request's Src for correlation.
type Response struct {
	ID     int             `json:"id"`
	Dst    string          `json:"dst,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

// RPCTopic returns the request topic for a reader's base topic (no trailing slash).
func RPCTopic(base string) string { return base + "/rpc" }

// StatusTopic returns the status topic for a reader's base topic (no trailing slash).
func StatusTopic(base string) string { return base + "/status" }

// ParseRequest decodes a JSON-RPC request frame.
func ParseRequest(b []byte) (Request, error) {
	var req Request
	err := json.Unmarshal(b, &req)
	return req, err
}

// NewResult builds a success response routed back to the request's Src.
func NewResult(req Request, result any) (Response, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return Response{}, err
	}
	return Response{
		ID:     req.ID,
		Dst:    req.Src,
		Result: raw,
	}, nil
}

// NewError builds an error response routed back to the request's Src.
func NewError(req Request, code int, msg string) Response {
	return Response{
		ID:  req.ID,
		Dst: req.Src,
		Error: &RPCError{
			Code:    code,
			Message: msg,
		},
	}
}

// Marshal encodes a response frame.
func (r Response) Marshal() ([]byte, error) { return json.Marshal(r) }
