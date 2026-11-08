// Package invocation enforces authentication assurance and transaction approval.
package invocation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
)

type StepUp struct {
	AcceptedACR []string `json:"acceptedAcrValues"`
	MaxAge      int64    `json:"maxAgeSeconds"`
}
type IdentityPolicy struct {
	Source  string   `json:"source"`
	Service string   `json:"service,omitempty"`
	Fields  []string `json:"fields,omitempty"`
}
type Rule struct {
	Retry            string          `json:"retry,omitempty"`    // empty inherits; never or once
	ReadOnly         *bool           `json:"readOnly,omitempty"` // operator classification, independent of retry
	Identity         *IdentityPolicy `json:"identity,omitempty"`
	StepUp           *StepUp         `json:"stepUp,omitempty"`
	ApprovalServices []string        `json:"approvalServices,omitempty"`
	// Only these top-level argument fields may leave the gateway as cleartext.
	ShareFields []string `json:"shareFields,omitempty"`
}
type UpstreamPolicy struct {
	Rule
	Tools    map[string]Rule    `json:"tools,omitempty"`
	Services map[string]Service `json:"services,omitempty"`
}
type Service struct {
	URL            string `json:"url"`
	Token          string `json:"token,omitempty"` // write-only in management responses
	HasToken       bool   `json:"hasToken,omitempty"`
	ApprovalOrigin string `json:"approvalOrigin"`
	CAPEM          string `json:"caPem,omitempty"`
}
type Settings struct {
	Revision int64              `json:"revision"`
	Services map[string]Service `json:"services"`
	// Stable upstream UUIDs, never aliases or caller-provided slugs.
	Upstreams map[string]UpstreamPolicy `json:"upstreams"`
}

// Service resolves a verifier or approval service in the upstream's scope.
func (s Settings) Service(upstream, name string) (Service, bool) {
	if service, ok := s.Upstreams[upstream].Services[name]; ok {
		return service, true
	}
	service, ok := s.Services[name]
	return service, ok
}

// HideCredentials redacts secrets from management responses and GitOps exports.
func (s Settings) HideCredentials(includeStatus bool) {
	hideServiceCredentials(s.Services, includeStatus)
	for _, policy := range s.Upstreams {
		hideServiceCredentials(policy.Services, includeStatus)
	}
}

func (p UpstreamPolicy) HideCredentials() {
	hideServiceCredentials(p.Services, true)
}

func hideServiceCredentials(services map[string]Service, includeStatus bool) {
	for name, service := range services {
		service.HasToken = includeStatus && service.Token != ""
		service.Token = ""
		services[name] = service
	}
}

func (p UpstreamPolicy) Empty() bool {
	return p.Retry == "" && p.ReadOnly == nil && p.Identity == nil && p.StepUp == nil && len(p.ApprovalServices) == 0 &&
		len(p.ShareFields) == 0 && len(p.Tools) == 0 && len(p.Services) == 0
}

var contextFieldPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

var acrPattern = regexp.MustCompile(`^[A-Za-z0-9:._/-]{1,200}$`)

func validateServices(services map[string]Service) error {
	if len(services) > 100 {
		return errors.New("too many invocation services")
	}
	for name, svc := range services {
		if !acrPattern.MatchString(name) {
			return errors.New("invalid service name")
		}
		u, e := url.Parse(svc.URL)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return fmt.Errorf("service %s requires an HTTPS URL", name)
		}
		o, e := url.Parse(svc.ApprovalOrigin)
		if e != nil || o.Scheme != "https" || o.Host == "" || o.User != nil || o.RawQuery != "" || o.Fragment != "" || (o.Path != "" && o.Path != "/") {
			return errors.New("approvalOrigin must be an HTTPS origin")
		}
		if len(svc.Token) > 8192 || len(svc.CAPEM) > 65536 {
			return errors.New("service credentials too large")
		}
	}
	return nil
}

func (s Settings) Validate() error {
	if len(s.Upstreams) > 10000 {
		return errors.New("too many invocation policies")
	}
	if err := validateServices(s.Services); err != nil {
		return err
	}
	validate := func(upstream string, r Rule) error {
		if r.Retry != "" && r.Retry != "never" && r.Retry != "once" {
			return errors.New("retry must be never or once")
		}
		if r.Identity != nil {
			switch r.Identity.Source {
			case "oauth_subject":
				if r.Identity.Service != "" || len(r.Identity.Fields) > 0 {
					return errors.New("oauth_subject cannot configure caller context")
				}
			case "caller_context":
				if _, ok := s.Service(upstream, r.Identity.Service); !ok {
					return errors.New("caller_context requires a configured identity service")
				}
				if len(r.Identity.Fields) == 0 || len(r.Identity.Fields) > 16 {
					return errors.New("caller_context requires 1 to 16 field names")
				}
				seen := map[string]bool{}
				for _, field := range r.Identity.Fields {
					if !contextFieldPattern.MatchString(field) || seen[field] {
						return errors.New("invalid or duplicate caller context field")
					}
					seen[field] = true
				}
			default:
				return errors.New("identity source must be oauth_subject or caller_context")
			}
		}
		if r.StepUp != nil {
			if len(r.StepUp.AcceptedACR) == 0 || len(r.StepUp.AcceptedACR) > 16 || r.StepUp.MaxAge < 1 || r.StepUp.MaxAge > 86400 {
				return errors.New("stepUp requires acceptedAcrValues and maxAgeSeconds between 1 and 86400")
			}
			for _, a := range r.StepUp.AcceptedACR {
				if !acrPattern.MatchString(a) {
					return errors.New("invalid ACR")
				}
			}
		}
		if len(r.ApprovalServices) > 8 || len(r.ShareFields) > 64 {
			return errors.New("too many services or shared fields")
		}
		for _, name := range r.ApprovalServices {
			if _, ok := s.Service(upstream, name); !ok {
				return fmt.Errorf("unknown approval service %s", name)
			}
		}
		for _, f := range r.ShareFields {
			if f == "" || len(f) > 200 {
				return errors.New("invalid shared field")
			}
		}
		return nil
	}
	for id, p := range s.Upstreams {
		if id == "" {
			return errors.New("upstream ID required")
		}
		if err := validateServices(p.Services); err != nil {
			return fmt.Errorf("upstream %s: %w", id, err)
		}
		for name := range p.Services {
			if _, exists := s.Services[name]; exists {
				return fmt.Errorf("upstream %s service %s conflicts with a shared service", id, name)
			}
		}
		if e := validate(id, p.Rule); e != nil {
			return fmt.Errorf("upstream %s: %w", id, e)
		}
		if (p.StepUp != nil || len(p.ApprovalServices) > 0) && p.Identity == nil {
			return errors.New("protected upstream requires an explicit identity source")
		}
		if len(p.Tools) > 10000 {
			return errors.New("too many tool policies")
		}
		for tool, r := range p.Tools {
			if tool == "" {
				return errors.New("tool name required")
			}
			if e := validate(id, r); e != nil {
				return fmt.Errorf("tool %s: %w", tool, e)
			}
			if effective, e := Merge(p.Rule, r); e != nil {
				return fmt.Errorf("tool %s: %w", tool, e)
			} else if (effective.StepUp != nil || len(effective.ApprovalServices) > 0) && effective.Identity == nil {
				return errors.New("protected tools require an explicit identity source")
			}
		}
	}
	return nil
}

// Merge is monotonic: approval services accumulate, ACRs intersect, age decreases.
// Shared fields intersect when both levels define an allowlist.
func Merge(a, b Rule) (Rule, error) {
	out := a
	if b.Retry != "" {
		out.Retry = b.Retry
	}
	if b.ReadOnly != nil {
		out.ReadOnly = b.ReadOnly
	}
	if a.Identity == nil {
		out.Identity = b.Identity
	} else if b.Identity != nil && !reflect.DeepEqual(a.Identity, b.Identity) {
		return Rule{}, errors.New("tool cannot replace inherited identity source, service or field mapping")
	}
	out.ApprovalServices = append([]string{}, a.ApprovalServices...)
	for _, s := range b.ApprovalServices {
		if !slices.Contains(out.ApprovalServices, s) {
			out.ApprovalServices = append(out.ApprovalServices, s)
		}
	}
	if a.StepUp == nil {
		out.StepUp = b.StepUp
	} else if b.StepUp != nil {
		st := &StepUp{MaxAge: min(a.StepUp.MaxAge, b.StepUp.MaxAge)}
		for _, v := range a.StepUp.AcceptedACR {
			if slices.Contains(b.StepUp.AcceptedACR, v) {
				st.AcceptedACR = append(st.AcceptedACR, v)
			}
		}
		if len(st.AcceptedACR) == 0 {
			return Rule{}, errors.New("inherited and tool ACR requirements have no intersection")
		}
		out.StepUp = st
	}
	if a.ShareFields == nil {
		out.ShareFields = b.ShareFields
	} else if b.ShareFields != nil {
		out.ShareFields = []string{}
		for _, v := range a.ShareFields {
			if slices.Contains(b.ShareFields, v) {
				out.ShareFields = append(out.ShareFields, v)
			}
		}
	}
	return out, nil
}
func (s Settings) Effective(upstream, tool string) (Rule, error) {
	p := s.Upstreams[upstream]
	return Merge(p.Rule, p.Tools[tool])
}

type Identity struct {
	Mode         string `json:"mode"`
	CredentialID string `json:"credentialId"`
	Issuer       string `json:"issuer,omitempty"`
	Subject      string `json:"subject,omitempty"`
	ACR          string `json:"acr,omitempty"`
	AuthTime     int64  `json:"authTime,omitempty"`
}

func ClaimsIdentity(mode, credential, issuer, subject string, claims map[string]any) Identity {
	id := Identity{Mode: mode, CredentialID: credential, Issuer: issuer, Subject: subject}
	id.ACR, _ = claims["acr"].(string)
	switch n := claims["auth_time"].(type) {
	case float64:
		if n > 0 && n < 1e12 && n == float64(int64(n)) {
			id.AuthTime = int64(n)
		}
	case json.Number:
		id.AuthTime, _ = n.Int64()
	}
	return id
}
func (s StepUp) Satisfied(id Identity, now time.Time) bool {
	return id.Mode == "oauth" && id.Subject != "" && slices.Contains(s.AcceptedACR, id.ACR) && id.AuthTime > 0 && id.AuthTime <= now.Unix()+30 && now.Unix()-id.AuthTime <= s.MaxAge
}
func (s StepUp) Challenge() string {
	return `Bearer error="insufficient_user_authentication", acr_values="` + strings.Join(s.AcceptedACR, " ") + `", max_age="` + fmt.Sprint(s.MaxAge) + `"`
}
