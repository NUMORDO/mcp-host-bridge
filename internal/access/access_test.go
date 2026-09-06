package access

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

func request(h http.Handler, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "https://bridge.example/mcp/server", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestBearer(t *testing.T) {
	c := &config.Config{Auth: config.Auth{TokenEnv: "BRIDGE_TEST_SECRET"}}
	t.Setenv(c.Auth.TokenEnv, "short")
	if _, e := Middleware(c); e == nil {
		t.Fatal("short credential accepted")
	}
	secret := strings.Repeat("a", 32)
	t.Setenv(c.Auth.TokenEnv, secret)
	wrap, e := Middleware(c)
	if e != nil {
		t.Fatal(e)
	}
	h := wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := auth.TokenInfoFromContext(r.Context()); got == nil || got.UserID != "static-bearer" {
			t.Error("missing stable identity")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, s := range []string{"", strings.Repeat("b", 32), secret + "x"} {
		w := request(h, s)
		if w.Code != 401 || w.Header().Get("WWW-Authenticate") == "" {
			t.Fatal("invalid credential accepted or challenge absent")
		}
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("credential leaked")
		}
	}
	if w := request(h, secret); w.Code != 204 {
		t.Fatalf("valid credential rejected: %d", w.Code)
	}
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Add("Authorization", "Bearer "+secret)
	r.Header.Add("Authorization", "Bearer "+secret)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("duplicate header accepted")
	}
	if w := request(Metadata(c), ""); w.Code != 404 {
		t.Fatal("static auth advertises OAuth")
	}
}
func fixture(t *testing.T) (*config.Config, *rsa.PrivateKey) {
	t.Helper()
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	b, e := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "public.pem")
	if e = os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: b}), 0600); e != nil {
		t.Fatal(e)
	}
	return &config.Config{PublicURL: "https://bridge.example", Auth: config.Auth{Issuer: "https://issuer.example", Audience: "https://bridge.example", PublicKeyFile: p, Scope: "mcp:read"}}, key
}
func validClaims() *claims {
	return &claims{RegisteredClaims: jwt.RegisteredClaims{Issuer: "https://issuer.example", Subject: "alice", Audience: jwt.ClaimStrings{"https://bridge.example"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)), NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Minute))}, Scope: "mcp:read other"}
}
func TestJWTValidation(t *testing.T) {
	c, key := fixture(t)
	wrap, e := Middleware(c)
	if e != nil {
		t.Fatal(e)
	}
	h := wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.TokenInfoFromContext(r.Context()).UserID != "alice" {
			t.Error("identity lost")
		}
		w.WriteHeader(204)
	}))
	tests := []struct {
		name string
		edit func(*claims)
		code int
	}{
		{"valid", func(*claims) {}, 204},
		{"expired", func(c *claims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute)) }, 401},
		{"missing expiration", func(c *claims) { c.ExpiresAt = nil }, 401},
		{"audience", func(c *claims) { c.Audience = jwt.ClaimStrings{"other"} }, 401},
		{"issuer", func(c *claims) { c.Issuer = "https://wrong.example" }, 401},
		{"not before", func(c *claims) { c.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Hour)) }, 401},
		{"subject", func(c *claims) { c.Subject = " " }, 401},
		{"scope", func(c *claims) { c.Scope = "other" }, 403},
		{"scope substring", func(c *claims) { c.Scope = "mcp:readwrite" }, 403},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cl := validClaims()
			tc.edit(cl)
			tok, e := jwt.NewWithClaims(jwt.SigningMethodRS256, cl).SignedString(key)
			if e != nil {
				t.Fatal(e)
			}
			w := request(h, tok)
			if w.Code != tc.code {
				t.Fatalf("status %d, expected %d", w.Code, tc.code)
			}
			if tc.code != 204 && !strings.Contains(w.Header().Get("WWW-Authenticate"), c.PublicURL+"/.well-known/oauth-protected-resource") {
				t.Fatal("metadata challenge missing")
			}
			if strings.Contains(w.Body.String(), tok) {
				t.Fatal("token leaked")
			}
		})
	}
	for _, method := range []jwt.SigningMethod{jwt.SigningMethodHS256, jwt.SigningMethodRS512} {
		var signingKey any = key
		if method.Alg() == "HS256" {
			signingKey = []byte("test-key")
		}
		tok, e := jwt.NewWithClaims(method, validClaims()).SignedString(signingKey)
		if e != nil {
			t.Fatal(e)
		}
		if request(h, tok).Code != 401 {
			t.Fatal("unexpected algorithm accepted")
		}
	}
	other, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	tok, e := jwt.NewWithClaims(jwt.SigningMethodRS256, validClaims()).SignedString(other)
	if e != nil {
		t.Fatal(e)
	}
	if request(h, tok).Code != 401 {
		t.Fatal("wrong signature accepted")
	}
	if request(h, "malformed-secret-token").Code != 401 {
		t.Fatal("malformed JWT accepted")
	}
}
func TestMetadata(t *testing.T) {
	c, _ := fixture(t)
	h := Metadata(c)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	var got struct {
		Resource string   `json:"resource"`
		Issuers  []string `json:"authorization_servers"`
		Scopes   []string `json:"scopes_supported"`
		Methods  []string `json:"bearer_methods_supported"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &got); e != nil {
		t.Fatal(e)
	}
	if w.Code != 200 || got.Resource != c.Auth.Audience || len(got.Issuers) != 1 || got.Issuers[0] != c.Auth.Issuer || len(got.Scopes) != 1 || got.Scopes[0] != "mcp:read" || len(got.Methods) != 1 || got.Methods[0] != "header" {
		t.Fatal("invalid metadata")
	}
	if strings.Contains(w.Body.String(), c.Auth.PublicKeyFile) {
		t.Fatal("private config path leaked")
	}
	for method, want := range map[string]int{"OPTIONS": 204, "POST": 405} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, "/", nil))
		if w.Code != want {
			t.Fatalf("%s status %d", method, w.Code)
		}
	}
}
func TestInvalidConfiguration(t *testing.T) {
	if _, e := Middleware(nil); e == nil {
		t.Fatal("nil accepted")
	}
	if _, e := Middleware(&config.Config{}); e == nil {
		t.Fatal("missing auth accepted")
	}
	c, _ := fixture(t)
	c.Auth.PublicKeyFile = filepath.Join(t.TempDir(), "missing")
	if _, e := Middleware(c); e == nil {
		t.Fatal("missing key accepted")
	}
	p := filepath.Join(t.TempDir(), "bad.pem")
	if e := os.WriteFile(p, []byte("not a key"), 0600); e != nil {
		t.Fatal(e)
	}
	c.Auth.PublicKeyFile = p
	if _, e := Middleware(c); e == nil {
		t.Fatal("invalid key accepted")
	}
}
