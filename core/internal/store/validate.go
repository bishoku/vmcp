package store

import (
	"encoding/json"
	"fmt"
	"github.com/bishoku/vmcp/core/internal/netpolicy"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/pii"
	"github.com/bishoku/vmcp/core/internal/policy"
)

var (
	slugRE       = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	toolNameRE   = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	headerNameRE = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")
)

// ValidSlug reports whether s is a legal slug.
func ValidSlug(s string) bool { return len(s) >= 1 && len(s) <= 63 && slugRE.MatchString(s) }

func fe(field, code, msg string) apierror.FieldError {
	return apierror.FieldError{Field: field, Code: code, Message: msg}
}

func vf(field, msg string) apierror.FieldError {
	return fe(field, apierror.CodeValidationFailed, msg)
}

// SlugErrors validates a slug at the given JSON pointer.
func SlugErrors(field, s string) []apierror.FieldError {
	if !ValidSlug(s) {
		return []apierror.FieldError{vf(field, "must be 1-63 characters: lowercase letters, digits and hyphens, not starting or ending with a hyphen")}
	}
	return nil
}

// ValidateUpstreamInput returns every problem with in.
func ValidateUpstreamInput(in UpstreamInput) []apierror.FieldError {
	var errs []apierror.FieldError
	if n := len(strings.TrimSpace(in.Name)); n < 1 || len(in.Name) > 120 {
		errs = append(errs, vf("/name", "must be 1-120 characters"))
	}
	if len(in.Description) > 1000 {
		errs = append(errs, vf("/description", "must be at most 1000 characters"))
	}
	if err := netpolicy.ValidateURL(in.URL); err != nil {
		errs = append(errs, vf("/url", "must be an absolute http(s) URL without credentials or fragments"))
	}
	if len(in.FallbackURLs) > 5 {
		errs = append(errs, vf("/fallbackUrls", "at most 5 fallback URLs"))
	}
	for i, f := range in.FallbackURLs {
		if err := netpolicy.ValidateURL(f); err != nil {
			errs = append(errs, vf(fmt.Sprintf("/fallbackUrls/%d", i), "must be an absolute http(s) URL without credentials or fragments"))
		} else if f == in.URL || slices.Index(in.FallbackURLs, f) != i {
			errs = append(errs, vf(fmt.Sprintf("/fallbackUrls/%d", i), "duplicates another URL of this upstream"))
		}
	}
	if in.Transport != "streamable_http" && in.Transport != "sse" && in.Transport != "stdio_worker" {
		errs = append(errs, vf("/transport", "must be one of: streamable_http, sse, stdio_worker"))
	}
	if in.Transport == "stdio_worker" {
		if !strings.HasPrefix(in.URL, "https://") || len(in.FallbackURLs) > 0 {
			errs = append(errs, vf("/url", "stdio worker requires a fixed HTTPS owner endpoint without fallbacks"))
		}
	}
	for k, v := range in.Headers {
		if !headerNameRE.MatchString(k) {
			errs = append(errs, vf("/headers/"+k, "invalid header name"))
		}
		if strings.ContainsAny(v, "\r\n") {
			errs = append(errs, vf("/headers/"+k, "value must not contain line breaks"))
		}
	}
	return errs
}

// ValidateVirtualServerInput checks the fields that do not need the database.
func ValidateVirtualServerInput(in VirtualServerInput) []apierror.FieldError {
	var errs []apierror.FieldError
	if n := len(strings.TrimSpace(in.Name)); n < 1 || len(in.Name) > 120 {
		errs = append(errs, vf("/name", "must be 1-120 characters"))
	}
	if len(in.Description) > 1000 {
		errs = append(errs, vf("/description", "must be at most 1000 characters"))
	}
	piiErrs := pii.Validate(in.PII)
	for _, k := range slices.Sorted(maps.Keys(piiErrs)) {
		errs = append(errs, vf("/pii"+k, piiErrs[k]))
	}
	exposed := map[string]bool{}
	for _, s := range in.Tools {
		exposed[config.ExposedName(s.Upstream, s.Tool, s.Alias)] = true
	}
	errs = append(errs, validateRateLimit(in.RateLimit)...)
	polErrs := policy.Validate(in.Policy, exposed)
	for _, k := range slices.Sorted(maps.Keys(polErrs)) {
		errs = append(errs, vf("/policy"+k, polErrs[k]))
	}
	for i, s := range in.Tools {
		p := fmt.Sprintf("/tools/%d", i)
		if !ValidSlug(s.Upstream) {
			errs = append(errs, vf(p+"/upstream", "must be a valid upstream slug"))
		}
		if s.Tool == "" {
			errs = append(errs, vf(p+"/tool", "is required"))
		}
		if s.Alias != "" && (len(s.Alias) > 128 || !toolNameRE.MatchString(s.Alias)) {
			errs = append(errs, vf(p+"/alias", "must be 1-128 characters of letters, digits, '_', '.' or '-'"))
		}
		if len(s.Description) > 2000 {
			errs = append(errs, vf(p+"/description", "must be at most 2000 characters"))
		}
	}
	return errs
}

// jsonEqual compares two JSON documents structurally.
func jsonEqual(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return string(a) == string(b)
	}
	return reflect.DeepEqual(x, y)
}
