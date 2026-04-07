package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	ActorHeader         = "X-Actor"
	ProjectScopesHeader = "X-Project-Scopes"
)

var ErrForbidden = errors.New("forbidden")

type contextKey string

const identityContextKey contextKey = "auth.identity"

type Identity struct {
	Actor      string  `json:"actor,omitempty"`
	ProjectIDs []int64 `json:"project_ids,omitempty"`
}

func NewIdentity(actor string, projectIDs []int64) Identity {
	return Identity{
		Actor:      strings.TrimSpace(actor),
		ProjectIDs: normalizeProjectIDs(projectIDs),
	}
}

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityContextKey, NewIdentity(identity.Actor, identity.ProjectIDs))
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	if ctx == nil {
		return Identity{}, false
	}
	identity, ok := ctx.Value(identityContextKey).(Identity)
	if !ok {
		return Identity{}, false
	}
	return identity, true
}

func (i Identity) AllowsProject(projectID int64) bool {
	if projectID <= 0 {
		return true
	}
	for _, candidate := range i.ProjectIDs {
		if candidate == projectID {
			return true
		}
	}
	return false
}

func RequireProjectAccess(ctx context.Context, projectID int64) error {
	identity, ok := IdentityFromContext(ctx)
	if !ok {
		return nil
	}
	if identity.AllowsProject(projectID) {
		return nil
	}
	return fmt.Errorf("%w: project %d is not within caller scope", ErrForbidden, projectID)
}

// IdentityMiddleware loads lightweight caller identity context for local or
// reverse-proxy deployments. `X-Project-Scopes` overrides the configured
// defaults, while `X-Actor` provides an optional display/audit actor name.
func IdentityMiddleware(defaultProjectIDs []int64) func(http.Handler) http.Handler {
	defaultProjectIDs = normalizeProjectIDs(defaultProjectIDs)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			projectIDs := defaultProjectIDs
			if headerProjectIDs := parseProjectIDsHeader(r.Header.Get(ProjectScopesHeader)); len(headerProjectIDs) > 0 {
				projectIDs = headerProjectIDs
			}
			actor := strings.TrimSpace(r.Header.Get(ActorHeader))
			if len(projectIDs) == 0 && actor == "" {
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), NewIdentity(actor, projectIDs))))
		})
	}
}

func parseProjectIDsHeader(raw string) []int64 {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	projectIDs := make([]int64, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || value <= 0 {
			continue
		}
		projectIDs = append(projectIDs, value)
	}
	return normalizeProjectIDs(projectIDs)
}

func normalizeProjectIDs(projectIDs []int64) []int64 {
	if len(projectIDs) == 0 {
		return nil
	}
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(projectIDs))
	for _, projectID := range projectIDs {
		if projectID <= 0 {
			continue
		}
		if _, ok := seen[projectID]; ok {
			continue
		}
		seen[projectID] = struct{}{}
		out = append(out, projectID)
	}
	return out
}
