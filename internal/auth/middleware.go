package auth

import (
	"net/http"

	v1alpha1 "github.com/TaliaMarine/k8s-driller/pkg/apis/driller/v1alpha1"
)

// RequireAuth rejects requests without a valid session and otherwise injects
// the Session into the request context for downstream handlers.
func (m *SessionManager) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, err := m.FromRequest(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r.WithContext(withSession(r.Context(), sess)))
	}
}

// RequireRole wraps RequireAuth and additionally rejects sessions that don't
// hold role (used for the admin-only endpoints in SPECS.md §6.1).
func (m *SessionManager) RequireRole(role v1alpha1.Role, next http.HandlerFunc) http.HandlerFunc {
	return m.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		sess, _ := SessionFromContext(r.Context())
		if sess.Role != role {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}

// AdminOrBootstrap allows a request through when either the caller holds an
// admin session, or presents the exact admin bootstrap token via the
// X-Driller-Admin-Token header. The token path exists solely to promote the
// first real admin (SPECS.md §4.1) — every routine role change should go
// through an existing admin's session instead.
func (m *SessionManager) AdminOrBootstrap(bootstrapToken string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token := r.Header.Get("X-Driller-Admin-Token"); token != "" && bootstrapToken != "" && token == bootstrapToken {
			next(w, r)
			return
		}
		m.RequireRole(v1alpha1.RoleAdmin, next)(w, r)
	}
}
