// Package access authenticates callers without forwarding their tokens upstream.
package access

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/NUMORDO/mcp-host-bridge/internal/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

type claims struct {
	jwt.RegisteredClaims
	Scope string `json:"scope"`
}

// Middleware supports a host-local static credential or externally issued JWTs.
// Static credentials represent one principal and require operator rotation.
func Middleware(c *config.Config) (func(http.Handler) http.Handler, error) {
	if c == nil {
		return nil, errors.New("authentication configuration required")
	}
	opts := &auth.RequireBearerTokenOptions{}
	var verify auth.TokenVerifier
	if c.Auth.TokenEnv != "" {
		if c.Auth.Issuer != "" || c.Auth.PublicKeyFile != "" {
			return nil, errors.New("ambiguous authentication mode")
		}
		secret := os.Getenv(c.Auth.TokenEnv)
		if len(secret) < 32 || strings.ContainsAny(secret, " \t\r\n") {
			return nil, errors.New("bearer secret must contain at least 32 bytes without whitespace")
		}
		expected := sha256.Sum256([]byte(secret))
		opts.AllowMissingExpiration = true
		verify = func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
			actual := sha256.Sum256([]byte(token))
			if subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
				return nil, auth.ErrInvalidToken
			}
			return &auth.TokenInfo{UserID: "static-bearer"}, nil
		}
	} else {
		if c.Auth.Issuer == "" || c.Auth.Audience == "" || c.Auth.PublicKeyFile == "" || len(strings.Fields(c.Auth.Scope)) == 0 || c.PublicURL == "" {
			return nil, errors.New("incomplete OAuth resource-server configuration")
		}
		data, err := os.ReadFile(c.Auth.PublicKeyFile)
		if err != nil {
			return nil, errors.New("cannot read OAuth public key")
		}
		key, err := jwt.ParseRSAPublicKeyFromPEM(data)
		if err != nil || key.N.BitLen() < 2048 {
			return nil, errors.New("OAuth requires an RSA public key of at least 2048 bits")
		}
		issuer, audience := c.Auth.Issuer, c.Auth.Audience
		opts.ResourceMetadataURL = c.PublicURL + "/.well-known/oauth-protected-resource"
		opts.Scopes = strings.Fields(c.Auth.Scope)
		verify = func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
			cl := &claims{}
			parsed, err := jwt.ParseWithClaims(token, cl, func(t *jwt.Token) (any, error) { return key, nil },
				jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(issuer), jwt.WithAudience(audience), jwt.WithExpirationRequired())
			if err != nil || !parsed.Valid || strings.TrimSpace(cl.Subject) == "" {
				return nil, auth.ErrInvalidToken
			}
			return &auth.TokenInfo{UserID: cl.Subject, Scopes: strings.Fields(cl.Scope), Expiration: cl.ExpiresAt.Time}, nil
		}
	}
	middleware := auth.RequireBearerToken(verify, opts)
	return func(next http.Handler) http.Handler {
		handler := middleware(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// A duplicated credential field must not be interpreted differently by proxies.
			if len(r.Header.Values("Authorization")) > 1 {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			if opts.ResourceMetadataURL == "" {
				w.Header().Set("WWW-Authenticate", "Bearer")
			}
			handler.ServeHTTP(w, r)
		})
	}, nil
}

// Metadata advertises the configured external issuer; it never issues tokens.
func Metadata(c *config.Config) http.Handler {
	if c == nil || c.Auth.TokenEnv != "" || c.Auth.Issuer == "" {
		return http.NotFoundHandler()
	}
	return auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
		Resource:               c.Auth.Audience,
		AuthorizationServers:   []string{c.Auth.Issuer},
		ScopesSupported:        strings.Fields(c.Auth.Scope),
		BearerMethodsSupported: []string{"header"},
	})
}
