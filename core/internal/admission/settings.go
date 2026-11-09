// Package admission configures pod-local overload protection, not distributed quotas.
package admission

import "fmt"

type Settings struct {
	Revision     int64 `json:"revision"`
	Total        int   `json:"total"`
	PerServer    int   `json:"perServer"`
	PerPrincipal int   `json:"perPrincipal"`
}

func DefaultSettings() Settings { return Settings{Total: 1024, PerServer: 128, PerPrincipal: 16} }
func (s Settings) Validate() error {
	if s.Total < 1 || s.Total > 65536 || s.PerServer < 1 || s.PerServer > s.Total || s.PerPrincipal < 1 || s.PerPrincipal > s.Total {
		return fmt.Errorf("total must be 1–65536; perServer and perPrincipal must be 1–total")
	}
	return nil
}
