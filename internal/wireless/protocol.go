// Package wireless implements the server side of calibre's "wireless device
// connection" (smart device app) protocol, so readers that can talk to calibre over
// Wi-Fi (Xteink "Calibre Transfer", KOReader, ...) can receive books from Lord Books.
//
// Wire format, both directions: the JSON array [opcode, {args}] preceded by its byte
// length in ASCII decimal, e.g. `34[0, {"key":"value"}]`. Book contents follow a
// SEND_BOOK message as raw bytes.
package wireless

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

const (
	opOK                    = 0
	opSetCalibreDeviceInfo  = 1
	opGetDeviceInformation  = 3
	opFreeSpace             = 5
	opGetBookCount          = 6
	opSendBooklists         = 7
	opSendBook              = 8
	opGetInitializationInfo = 9
	opNoop                  = 12
	opDeleteBook            = 13
	opGetBookFileSegment    = 14
	opSendBookMetadata      = 16
	opDisplayMessage        = 17
	opCalibreBusy           = 18
	opSetLibraryInfo        = 19
	opError                 = 20
)

// broadcastPorts are where calibre listens for discovery packets; clients try them in order.
var broadcastPorts = []int{54982, 48123, 39001, 44044, 59678}

const clientString = "calibre wireless device client"

type message struct {
	Op   int
	Args map[string]any
}

func writeMessage(w io.Writer, op int, args map[string]any) error {
	if args == nil {
		args = map[string]any{}
	}
	body, err := json.Marshal([]any{op, args})
	if err != nil {
		return err
	}
	_, err = w.Write(append([]byte(strconv.Itoa(len(body))), body...))
	return err
}

// Limits on what a reader can make us do. Anything on the network can connect while
// the server is on, so each value a peer sends is bounded before it sizes anything.
const (
	// maxMessage bounds a single JSON message (metadata with a thumbnail is far smaller).
	maxMessage = 16 << 20
	// maxBooksOnDevice bounds the book count a reader reports.
	maxBooksOnDevice = 100_000
	// maxFetchBytes bounds a book copied back from a reader.
	maxFetchBytes = 500 << 20
)

func readMessage(r *bufio.Reader) (message, error) {
	var digits []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return message{}, err
		}
		if b == '[' {
			if err := r.UnreadByte(); err != nil {
				return message{}, err
			}
			break
		}
		if b < '0' || b > '9' || len(digits) > 9 {
			return message{}, fmt.Errorf("protocol error: unexpected byte %q before message", b)
		}
		digits = append(digits, b)
	}
	n, err := strconv.Atoi(string(digits))
	if err != nil || n <= 0 || n > maxMessage {
		return message{}, fmt.Errorf("protocol error: bad message length %q", digits)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return message{}, err
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(buf, &raw); err != nil || len(raw) < 1 {
		return message{}, fmt.Errorf("protocol error: bad json: %v", err)
	}
	var m message
	if err := json.Unmarshal(raw[0], &m.Op); err != nil {
		return message{}, fmt.Errorf("protocol error: bad opcode: %v", err)
	}
	m.Args = map[string]any{}
	if len(raw) > 1 {
		// Some clients send null or a non-object for empty replies.
		_ = json.Unmarshal(raw[1], &m.Args)
		if m.Args == nil {
			m.Args = map[string]any{}
		}
	}
	return m, nil
}

func argString(m map[string]any, k string) string {
	if s, ok := m[k].(string); ok {
		return s
	}
	return ""
}

func argBool(m map[string]any, k string) bool {
	b, _ := m[k].(bool)
	return b
}

// argInt reads a number from decoded JSON (float64) or from maps we built ourselves.
func argInt(m map[string]any, k string) int64 {
	switch v := m[k].(type) {
	case float64:
		if v != v || v > 1<<62 || v < -(1<<62) { // NaN or out of int64 range
			return 0
		}
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}

func argStrings(m map[string]any, k string) []string {
	list, _ := m[k].([]any)
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
