package store

import "github.com/bishoku/vmcp/core/internal/apierror"

// MaxRateLimit bounds a configured limit to something that is still a limit.
const MaxRateLimit = 1_000_000

func validateRateLimit(v *int) []apierror.FieldError {
	if v != nil && (*v < 1 || *v > MaxRateLimit) {
		return []apierror.FieldError{vf("/rateLimitPerMinute", "must be between 1 and 1000000, or null for no limit")}
	}
	return nil
}

func equalInt(a, b *int) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
