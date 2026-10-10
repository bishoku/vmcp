package netpolicy

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Origins accepts only explicitly configured browser origins. Native clients
// may omit Origin; an absent allowlist does not allow arbitrary browser origins.
func Origins() (map[string]bool, error) {
	origins := map[string]bool{}
	for _, raw := range split(os.Getenv("VMCP_ALLOWED_ORIGINS")) {
		origin, err := CanonicalOrigin(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid VMCP_ALLOWED_ORIGINS entry %q", raw)
		}
		origins[origin] = true
	}
	return origins, nil
}
func CanonicalOrigin(raw string) (string, error) {
	if err := ValidateURL(raw); err != nil {
		return "", err
	}
	u, _ := url.Parse(raw)
	if u.Path != "" || u.RawQuery != "" || u.ForceQuery {
		return "", fmt.Errorf("origin must contain only scheme and authority")
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	port := u.Port()
	if port != "" && !((u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80")) {
		host += ":" + port
	}
	return strings.ToLower(u.Scheme) + "://" + host, nil
}
