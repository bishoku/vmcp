package controlplane

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/mockupstream"
	"github.com/bishoku/vmcp/core/internal/store"
)

func (s *Server) mockProvider(w http.ResponseWriter, r *http.Request) error {
	return writeJSON(w, 200, map[string]any{"enabled": s.Store.WireMockURL != "", "url": s.Store.WireMockURL})
}

// Read one repeatable snapshot; HTTP reconciliation never holds a database
// transaction or the configuration revision lock.
func (s *Server) desiredMocks(ctx context.Context) ([]mockupstream.Mapping, string, int64, error) {
	out := []mockupstream.Mapping{}
	var owner string
	var revision int64
	err := s.Store.View(ctx, func(tx *store.Tx) error {
		var err error
		owner, err = tx.MockOwner(ctx)
		if err != nil {
			return err
		}
		revision, err = tx.Revision(ctx)
		if err != nil {
			return err
		}
		after := ""
		for {
			items, more, err := tx.ListUpstreams(ctx, after, 200, nil)
			if err != nil {
				return err
			}
			for _, u := range items {
				if u.Kind != "rest" || u.REST == nil || u.REST.Mock == nil || !u.Enabled {
					continue
				}
				if u.URL != s.Store.WireMockURL {
					return errors.New("saved mock endpoint differs from VMCP_WIREMOCK_URL; save the mock upstream again")
				}
				out = append(out, mockupstream.Mappings(owner, *u.REST.Mock, u.REST.MockOperations())...)
			}
			if !more {
				return nil
			}
			after = items[len(items)-1].Slug
		}
	})
	return out, owner, revision, err
}
func (s *Server) syncMocks(ctx context.Context, namespace string) error {
	if s.Store.WireMockURL == "" {
		return errors.New("WireMock integration is disabled")
	}
	desired, owner, revision, err := s.desiredMocks(ctx)
	if err != nil {
		return err
	}
	// Coordinate replicas without taking the configuration revision lock.
	conn, err := s.Store.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(784321960)`).Scan(&acquired); err != nil {
		return err
	}
	if !acquired {
		return errors.New("another control plane is applying mock definitions; retry shortly")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := conn.Exec(cleanup, `SELECT pg_advisory_unlock(784321960)`); err != nil {
			_ = conn.Conn().Close(cleanup)
		}
	}()
	var current int64
	if err := conn.QueryRow(ctx, `SELECT revision FROM config_revision`).Scan(&current); err != nil {
		return err
	}
	if current != revision {
		return errors.New("configuration changed before mock publication; retry shortly")
	}
	client := mockupstream.NewClient(s.Store.WireMockURL)
	if namespace != "" {
		return client.ReconcileNamespace(ctx, owner, desired, namespace)
	}
	return client.Reconcile(ctx, owner, desired)
}
func (s *Server) RunMockUpstreams(ctx context.Context) {
	if s.Store.WireMockURL == "" {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := s.syncMocks(bounded, "")
		cancel()
		if err != nil && ctx.Err() == nil {
			s.Log.Warn("mock upstream reconciliation failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) mockAction(w http.ResponseWriter, r *http.Request) error {
	conn, err := s.Store.Reader().UpstreamConnInfo(r.Context(), r.PathValue("slug"))
	if err != nil {
		return err
	}
	if conn.REST == nil || conn.REST.Mock == nil {
		return apierror.Validation([]apierror.FieldError{storeRestError("mock upstream required")})
	}
	if r.Method == "POST" {
		if r.PathValue("action") != "sync" {
			return apierror.NotFound("mock action", r.PathValue("action"))
		}
		p, _ := r.Context().Value(principalKey{}).(*principal)
		if p == nil || p.Role != "admin" {
			return apierror.Forbidden("Applying mock definitions requires admin access")
		}
		match, err := parseIfMatch(r)
		if err != nil {
			return err
		}
		if match == nil || *match != conn.Version {
			return apierror.PreconditionFailed()
		}
		bounded, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := s.syncMocks(bounded, conn.REST.Mock.Namespace); err != nil {
			return apierror.New(502, apierror.CodeUpstreamUnreach, "Mock definitions could not be applied", err.Error())
		}
	}
	if s.Store.WireMockURL == "" {
		return writeJSON(w, 200, map[string]any{"state": "disabled", "message": "Mock management is disabled. Configure VMCP_WIREMOCK_URL; disable the upstream to stop already configured tool calls"})
	}
	bounded, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	actual, err := mockupstream.NewClient(s.Store.WireMockURL).List(bounded)
	if err != nil {
		return writeJSON(w, 200, map[string]any{"state": "unavailable", "message": err.Error()})
	}
	owner, err := s.Store.Reader().MockOwner(r.Context())
	if err != nil {
		return err
	}
	expected := mockupstream.Mappings(owner, *conn.REST.Mock, conn.REST.MockOperations())
	if !conn.Enabled {
		expected = nil
	}
	present := map[string]mockupstream.Metadata{}
	for _, m := range actual {
		if m.Managed().Namespace == conn.REST.Mock.Namespace {
			present[m.ID] = m.Managed()
		}
	}
	state := "active"
	if !conn.Enabled {
		state = "disabled"
	}
	if len(present) != len(expected) {
		state = "pending"
	} else {
		for _, m := range expected {
			if present[m.ID] != m.Metadata["vmcp"] {
				state = "pending"
				break
			}
		}
	}
	return writeJSON(w, 200, map[string]any{"state": state, "ruleCount": len(expected), "namespace": conn.REST.Mock.Namespace, "message": "Definitions are saved in vMCP and reconciled to WireMock every five seconds"})
}
