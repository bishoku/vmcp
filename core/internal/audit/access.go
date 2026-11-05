package audit

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

// AccessDomain is a snapshot-owned identity, independent of mutable slugs or live
// ownership rows. Provider evidence never grants access to consumer messages.
type AccessDomain struct {
	Kind         string
	ResourceKind string
	ResourceID   string
	ProjectID    string
	OwnerTeamID  string
	EpochID      string
}
type AccessContext struct {
	Tenant         string
	Classification string
	Origin         string
	Revision       int64
	Domains        []AccessDomain
}

func (c *AccessContext) Clone() *AccessContext {
	if c == nil {
		return nil
	}
	out := *c
	out.Domains = append([]AccessDomain(nil), c.Domains...)
	return &out
}
func (c *AccessContext) validate() error {
	if c == nil {
		return nil
	}
	if c.Tenant == "" || c.Origin == "" || len(c.Tenant) > 2048 || len(c.Origin) > 2048 || len(c.Domains) > 16 || c.Revision <= 0 {
		return errors.New("invalid audit provenance")
	}
	if c.Classification == "tenant" && len(c.Domains) == 0 {
		return nil
	}
	if c.Classification != "resource" || len(c.Domains) == 0 {
		return errors.New("invalid audit provenance classification")
	}
	consumers := false
	for _, d := range c.Domains {
		if d.Kind != "consumer" && d.Kind != "provider" && d.Kind != "target" {
			return errors.New("invalid audit domain")
		}
		if d.Kind != "provider" {
			consumers = true
		}
		if d.ResourceKind != "upstream" && d.ResourceKind != "virtual_server" {
			return errors.New("invalid audit resource kind")
		}
		for _, id := range []string{d.ResourceID, d.EpochID} {
			if !ValidID(id) {
				return errors.New("invalid audit resource identity")
			}
		}
		for _, id := range []string{d.ProjectID, d.OwnerTeamID} {
			if id != "" && !ValidID(id) {
				return errors.New("invalid audit ownership identity")
			}
		}
	}
	if !consumers {
		return errors.New("audit context lacks access domain")
	}
	return nil
}

type accessCommand struct {
	sql  string
	args []any
}

func accessCommands(e Event) []accessCommand {
	if e.Access == nil {
		return nil
	}
	c := e.Access
	commands := []accessCommand{{`INSERT INTO audit_access_contexts(event_id,tenant_id,classification,origin,operation_config_revision,provenance_state) VALUES($1,$2,$3,$4,$5,'verified')`, []any{e.EventID, c.Tenant, c.Classification, c.Origin, c.Revision}}}
	for _, d := range c.Domains {
		commands = append(commands, accessCommand{`INSERT INTO audit_access_domains(event_id,domain_kind,resource_kind,resource_id,project_id,owner_team_id,epoch_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, []any{e.EventID, d.Kind, d.ResourceKind, d.ResourceID, nullable(d.ProjectID), nullable(d.OwnerTeamID), d.EpochID}})
	}
	return commands
}
func insertAccess(ctx context.Context, q Sink, e Event) error {
	for _, cmd := range accessCommands(e) {
		if _, err := q.Exec(ctx, cmd.sql, cmd.args...); err != nil {
			return err
		}
	}
	return nil
}
func queueAccess(b *pgx.Batch, e Event) {
	for _, cmd := range accessCommands(e) {
		b.Queue(cmd.sql, cmd.args...)
	}
}

// WithProvider pins the actual adapter's provider identity without replacing the
// consumer scope or querying ownership during dispatch.
func WithProvider(ctx context.Context, slug string, d AccessDomain) context.Context {
	if op, ok := ctx.Value(operationKey{}).(operationContext); ok {
		op.base.Upstream = slug
		if op.base.Access != nil && op.base.Access.Classification == "resource" && d.EpochID != "" {
			c := op.base.Access.Clone()
			domains := c.Domains[:0]
			for _, existing := range c.Domains {
				if existing.Kind != "provider" {
					domains = append(domains, existing)
				}
			}
			d.Kind = "provider"
			c.Domains = append(domains, d)
			op.base.Access = c
		}
		return context.WithValue(ctx, operationKey{}, op)
	}
	return ctx
}

// Account for operation-owned provenance in both bounded writer queues.
func eventQueueBytes(e Event) int64 {
	n := int64(len(e.Ciphertext) + 4096)
	if e.Access != nil {
		n += int64(len(e.Access.Tenant) + len(e.Access.Origin) + 128 + len(e.Access.Domains)*256)
	}
	return n
}
