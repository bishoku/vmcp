package securitydemo

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/bishoku/vmcp/core/internal/invocation"
)

type customerSession struct {
	User       invocation.Identity
	CustomerID string
	ExpiresAt  time.Time
}

func (s *Server) customerSessions(w http.ResponseWriter, r *http.Request) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		http.Error(w, "customer login required", 401)
		return
	}
	id, err := s.CustomerVerifier.Verify(r.Context(), token)
	if err != nil || id.Claims["azp"] != "vmcp-customer-demo" {
		http.Error(w, "customer login required", 401)
		return
	}
	var body struct {
		SessionID   string `json:"sessionId"`
		ChallengeID string `json:"challengeId"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil {
		http.Error(w, "invalid session request", 400)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.customers == nil {
		s.customers = map[string]*customerSession{}
	}
	if body.ChallengeID != "" {
		body.SessionID = s.challenges[body.ChallengeID]
		if body.SessionID == "" {
			http.Error(w, "challenge unavailable", 403)
			return
		}
	}
	if body.SessionID != "" {
		old := s.customers[body.SessionID]
		if old == nil || old.User.Subject != id.Subject || !old.ExpiresAt.After(time.Now()) {
			http.Error(w, "session unavailable for this customer", 403)
			return
		}
	} else {
		if len(s.customers) >= 10000 {
			http.Error(w, "session capacity exceeded", 503)
			return
		}
		body.SessionID = rand.Text()
	}
	name, _ := id.Claims["preferred_username"].(string)
	if name == "" {
		name = id.Subject
	}
	user := invocation.ClaimsIdentity("external", "", s.CustomerVerifier.Issuer(), id.Subject, id.Claims)
	if old := s.customers[body.SessionID]; old != nil && (old.User.AuthTime > user.AuthTime || (old.User.AuthTime == user.AuthTime && old.User.ACR == "2" && user.ACR != "2")) {
		user = old.User
	}
	s.customers[body.SessionID] = &customerSession{User: user, CustomerID: name, ExpiresAt: time.Now().Add(15 * time.Minute)}
	reply(w, map[string]any{"sessionId": body.SessionID, "customerId": name, "user": user, "expiresAt": s.customers[body.SessionID].ExpiresAt})
}

// callerCustomer is called under s.mu. Values from callerContext are never
// accepted as identity without a lookup in the application's session store.
func (s *Server) callerCustomer(values map[string]string) (*customerSession, bool) {
	c := s.customers[values["sessionId"]]
	return c, c != nil && c.ExpiresAt.After(time.Now()) && c.CustomerID == values["customerId"]
}
func customerAssurance(step *invocation.StepUp, c *customerSession) bool {
	if step == nil {
		return true
	}
	user := c.User
	user.Mode = "oauth"
	return step.Satisfied(user, time.Now())
}
