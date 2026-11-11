package invocation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http/httpguts"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// All credentials live in the encrypted invocation-security document.
type ServiceOAuth struct {
	TokenURL        string   `json:"tokenUrl"`
	ClientID        string   `json:"clientId"`
	ClientSecret    string   `json:"clientSecret,omitempty"`
	HasClientSecret bool     `json:"hasClientSecret,omitempty"`
	Scopes          []string `json:"scopes,omitempty"`
	Audience        string   `json:"audience,omitempty"`
	AuthStyle       string   `json:"authStyle,omitempty"`
}

func (o ServiceOAuth) AuthenticationStyle() string {
	if o.AuthStyle == "" {
		return "client_secret_basic"
	}
	return o.AuthStyle
}
func (s Service) Authentication() string {
	if s.AuthType == "" {
		return "bearer"
	}
	return s.AuthType
}
func (s Service) Timeout() time.Duration {
	if s.TimeoutSeconds == 0 {
		return 2 * time.Second
	}
	return time.Duration(s.TimeoutSeconds) * time.Second
}
func (s Service) Redacted(status bool) Service {
	s.HasToken = status && s.Token != ""
	s.Token = ""
	s.HasHeaders = nil
	if len(s.Headers) > 0 {
		headers := make(map[string]string, len(s.Headers))
		if status {
			s.HasHeaders = map[string]bool{}
		}
		for name, value := range s.Headers {
			headers[name] = ""
			if status {
				s.HasHeaders[name] = value != ""
			}
		}
		s.Headers = headers
	}
	if s.OAuth != nil {
		o := *s.OAuth
		o.HasClientSecret = status && o.ClientSecret != ""
		o.ClientSecret = ""
		s.OAuth = &o
	}
	return s
}
func (s Service) ValidateConnection() error {
	if s.TimeoutSeconds < 0 || s.TimeoutSeconds > 60 {
		return errors.New("timeoutSeconds must be 1 to 60, or 0 for the 2 second default")
	}
	switch s.Authentication() {
	case "bearer":
		if s.OAuth != nil || len(s.Headers) > 0 {
			return errors.New("bearer authentication cannot include OAuth or custom headers")
		}
		if !httpguts.ValidHeaderFieldValue(s.Token) {
			return errors.New("invalid service token")
		}
	case "none":
		if s.Token != "" || s.OAuth != nil || len(s.Headers) > 0 {
			return errors.New("unauthenticated services cannot include credentials")
		}
	case "headers":
		if s.Token != "" || s.OAuth != nil || len(s.Headers) == 0 || len(s.Headers) > 16 {
			return errors.New("header authentication requires 1 to 16 headers and no other credentials")
		}
		seen := map[string]bool{}
		for name, value := range s.Headers {
			key := strings.ToLower(name)
			if !httpguts.ValidHeaderFieldName(name) || seen[key] {
				return errors.New("invalid or duplicate authentication header")
			}
			seen[key] = true
			switch key {
			case "host", "content-type", "content-length", "connection", "transfer-encoding", "trailer", "upgrade", "te", "expect", "proxy-authorization", "proxy-authenticate", "keep-alive":
				return fmt.Errorf("header %s is reserved", name)
			}
			if strings.HasPrefix(key, "sec-") || strings.HasPrefix(key, "proxy-") || len(value) > 8192 || !httpguts.ValidHeaderFieldValue(value) {
				return errors.New("invalid authentication header value or name")
			}
		}
	case "oauth_client_credentials":
		if s.Token != "" || len(s.Headers) > 0 || s.OAuth == nil {
			return errors.New("OAuth authentication requires only an oauth configuration")
		}
		o := s.OAuth
		u, err := url.Parse(o.TokenURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return errors.New("OAuth tokenUrl must be HTTPS without credentials or fragment")
		}
		if o.ClientID == "" || len(o.ClientID) > 2048 || len(o.ClientSecret) > 8192 || len(o.Audience) > 2048 {
			return errors.New("invalid OAuth client credentials")
		}
		if o.AuthStyle != "" && o.AuthStyle != "client_secret_basic" && o.AuthStyle != "client_secret_post" {
			return errors.New("invalid OAuth authStyle")
		}
		if len(o.Scopes) > 50 {
			return errors.New("too many OAuth scopes")
		}
		for _, scope := range o.Scopes {
			if scope == "" || len(scope) > 2048 || strings.ContainsAny(scope, " \t\r\n\"\\") {
				return errors.New("invalid OAuth scope")
			}
		}
	default:
		return errors.New("unsupported service authType")
	}
	return nil
}

// RetainCredentials never carries secrets to a changed destination or identity.
func (s Service) RetainCredentials(old Service) (Service, error) {
	same := s.URL == old.URL && s.CAPEM == old.CAPEM && s.Authentication() == old.Authentication()
	switch s.Authentication() {
	case "bearer":
		if s.Token == "" && same {
			s.Token = old.Token
		}
		if s.Token == "" {
			return s, errors.New("explicit service token required for new or changed endpoint/authentication")
		}
	case "headers":
		headers := map[string]string{}
		for name, value := range s.Headers {
			if value == "" && same {
				for oldName, oldValue := range old.Headers {
					if strings.EqualFold(name, oldName) {
						value = oldValue
						break
					}
				}
			}
			if value == "" {
				return s, errors.New("explicit values required for new or changed authentication headers")
			}
			headers[http.CanonicalHeaderKey(name)] = value
		}
		// Validate before canonicalization so case-insensitive duplicates cannot disappear.
		if err := s.ValidateConnection(); err != nil {
			return s, err
		}
		s.Headers = headers
	case "oauth_client_credentials":
		if s.OAuth != nil {
			o := *s.OAuth
			if o.ClientSecret == "" && same && old.OAuth != nil && o.TokenURL == old.OAuth.TokenURL && o.ClientID == old.OAuth.ClientID && o.AuthenticationStyle() == old.OAuth.AuthenticationStyle() {
				o.ClientSecret = old.OAuth.ClientSecret
			}
			if o.ClientSecret == "" {
				return s, errors.New("explicit OAuth client secret required for new or changed endpoint/client")
			}
			o.HasClientSecret = false
			s.OAuth = &o
		}
	}
	s.HasToken = false
	s.HasHeaders = nil
	return s, s.ValidateConnection()
}

type serviceToken struct {
	gate  chan struct{}
	token string
	until time.Time
}

var serviceTokens = struct {
	sync.Mutex
	entries map[[32]byte]*serviceToken
}{entries: map[[32]byte]*serviceToken{}}

func tokenSource(s Service) *serviceToken {
	raw, _ := json.Marshal(s)
	key := sha256.Sum256(raw)
	serviceTokens.Lock()
	defer serviceTokens.Unlock()
	if source := serviceTokens.entries[key]; source != nil {
		return source
	}
	if len(serviceTokens.entries) >= 256 {
		for old := range serviceTokens.entries {
			delete(serviceTokens.entries, old)
			break
		}
	}
	source := &serviceToken{gate: make(chan struct{}, 1)}
	serviceTokens.entries[key] = source
	return source
}
func (s *serviceToken) get(ctx context.Context, svc Service, client *http.Client) (string, error) {
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-s.gate }()
	if time.Now().Before(s.until) {
		return s.token, nil
	}
	o := svc.OAuth
	style := oauth2.AuthStyleInHeader
	if o.AuthStyle == "client_secret_post" {
		style = oauth2.AuthStyleInParams
	}
	cfg := clientcredentials.Config{ClientID: o.ClientID, ClientSecret: o.ClientSecret, TokenURL: o.TokenURL, Scopes: o.Scopes, AuthStyle: style}
	if o.Audience != "" {
		cfg.EndpointParams = url.Values{"audience": {o.Audience}}
	}
	token, err := cfg.Token(context.WithValue(ctx, oauth2.HTTPClient, client))
	if err != nil || token == nil || token.AccessToken == "" || !strings.EqualFold(token.Type(), "Bearer") || !httpguts.ValidHeaderFieldValue(token.AccessToken) {
		return "", errors.New("approval OAuth token unavailable")
	}
	ttl := time.Until(token.Expiry)
	if token.Expiry.IsZero() {
		ttl = 5 * time.Minute
	}
	if ttl <= 0 {
		return "", errors.New("approval OAuth token expired")
	}
	margin := min(30*time.Second, ttl/4)
	s.token = token.AccessToken
	s.until = time.Now().Add(ttl - margin)
	return s.token, nil
}
func (s *serviceToken) invalidate(ctx context.Context, token string) {
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-s.gate }()
	if s.token == token {
		s.until = time.Time{}
	}
}
