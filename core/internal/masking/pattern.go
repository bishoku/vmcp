// Package masking validates operator-supplied masking expressions.
package masking

import (
	"errors"
	"fmt"
	"regexp"
)

func Compile(pattern string) (*regexp.Regexp, error) {
	if len(pattern) == 0 || len(pattern) > 500 {
		return nil, errors.New("regex must be 1-500 bytes")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid RE2 regular expression: %w", err)
	}
	if re.MatchString("") {
		return nil, errors.New("must not match the empty string")
	}
	return re, nil
}
