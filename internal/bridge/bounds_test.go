package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOutputByteLimits(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		limit      int64
		line, fail bool
	}{
		{"valid frames", "123\n456\n", 4, true, false},
		{"too long frame", "12345\n", 4, true, true},
		{"unterminated frame", strings.Repeat("x", 10000), 1024, true, true},
		{"total exact", "1234", 4, false, false},
		{"total too long", "12345", 4, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var r io.Reader = &totalBound{ReadCloser: io.NopCloser(strings.NewReader(tc.body)), remaining: tc.limit}
			if tc.line {
				r = &lineBound{ReadCloser: io.NopCloser(strings.NewReader(tc.body)), limit: tc.limit}
			}
			b, e := io.ReadAll(r)
			if tc.fail != errors.Is(e, errOutputLimit) {
				t.Fatalf("got %v", e)
			}
			if tc.fail && int64(len(b)) > tc.limit {
				t.Fatal("oversized input reached decoder")
			}
		})
	}
}
func TestHTTPOutputLimitChunked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, strings.Repeat("x", 8192))
	}))
	defer server.Close()
	client := &http.Client{Transport: boundedHTTP{base: http.DefaultTransport, limit: 1024}}
	r, e := client.Get(server.URL)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	b, e := io.ReadAll(r.Body)
	if !errors.Is(e, errOutputLimit) || len(b) > 1024 {
		t.Fatal("chunked limit not enforced before decode")
	}
}
func TestModernBackendRejected(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "modern", Version: "1"}, nil)
	h := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{Stateless: true}))
	defer h.Close()
	_, e := dial(context.Background(), config.Definition{Type: "http", URL: h.URL}, 1024*1024)
	if e == nil || !strings.Contains(e.Error(), "sessionless") {
		t.Fatalf("modern upstream must fail closed: %v", e)
	}
}

func BenchmarkLineBound(b *testing.B) {
	payload := "\"" + strings.Repeat("x", 8189) + "\"\n"
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for b.Loop() {
		r := &lineBound{ReadCloser: io.NopCloser(strings.NewReader(payload)), limit: 16384}
		_, _ = io.Copy(io.Discard, r)
	}
}

func TestMultilineJSONCannotBypassFrameLimit(t *testing.T) {
	payload := "{\n\"x\":[\n" + strings.Repeat("0,\n", 10000) + "0]\n}\n"
	r := &lineBound{ReadCloser: io.NopCloser(strings.NewReader(payload)), limit: 1024}
	var raw json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err == nil {
		t.Fatal("multiline frame bypassed limit")
	}
	if len(raw) > 1024 {
		t.Fatal("oversized message reached SDK-equivalent decoder")
	}
}
func BenchmarkPolicyLookup(b *testing.B) {
	names := []string{"search", "read_note", "get_metadata", "query_docs", "list_schemas"}
	b.ReportAllocs()
	for b.Loop() {
		if !allowed(names, "query_docs") {
			b.Fatal("missing")
		}
	}
}
