// Package config holds process-level settings read from the environment.
package config

import (
	"fmt"
	"net/netip"
	"os"
	"strings"
)

// Env returns the value of key, or fallback when unset or empty.
func Env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// MustEnv returns the value of key or an error when it is unset.
func MustEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("required environment variable %s is not set", key)
	}
	return v, nil
}

// ParsePrefixes parses a comma-separated list of CIDRs or addresses ("" yields none).
func ParsePrefixes(list string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, f := range strings.Split(list, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if p, err := netip.ParsePrefix(f); err == nil {
			out = append(out, p)
		} else if a, err := netip.ParseAddr(f); err == nil {
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		} else {
			return nil, fmt.Errorf("invalid address or CIDR %q", f)
		}
	}
	return out, nil
}
