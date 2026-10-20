package restapi

// ExecutionError retains a transport cause for structured operator diagnostics.
// Its public text deliberately excludes the cause, which may contain secrets.
type ExecutionError struct {
	Stage string
	cause error
}

func (e *ExecutionError) Error() string {
	if e.Stage == "tls_configuration" {
		return "REST TLS configuration is invalid"
	}
	return "REST request failed or timed out; operation may have completed"
}

func (e *ExecutionError) Unwrap() error { return e.cause }
