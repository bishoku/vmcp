package controlplane

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/bishoku/vmcp/core/internal/apierror"
)

type failedControlBody struct{}

func (failedControlBody) Read([]byte) (int, error) {
	return 0, errors.New("private transport diagnostic")
}
func (failedControlBody) Close() error { return nil }

func TestControlAuditBodyReadFailureIsClientError(t *testing.T) {
	for _, capture := range []bool{false, true} {
		req := httptest.NewRequest("PATCH", "/api/v1/upstreams/crm", nil)
		req.Pattern = "PATCH /api/v1/upstreams/{slug}"
		req.Body = failedControlBody{}
		_, _, err := (&Server{}).pinControlAudit(req, capture)
		var problem *apierror.Error
		if !errors.As(err, &problem) || problem.Status != 400 || problem.Code != "invalid_body" || problem.Detail != "request could not be read" {
			t.Fatal("body failure exposed details or became audit outage", err)
		}
	}
}
