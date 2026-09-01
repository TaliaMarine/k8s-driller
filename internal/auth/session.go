// Package auth implements OIDC login (SPECS.md §4.1 Auth module), signed
// session cookies, and the admin bootstrap-token break-glass path used only
// to promote the first real admin.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	v1alpha1 "github.com/TaliaMarine/k8s-driller/pkg/apis/driller/v1alpha1"
)

const sessionCookieName = "driller_session"
const sessionTTL = 12 * time.Hour

// Session is the authenticated user carried in the signed cookie.
type Session struct {
	Subject   string        `json:"sub"`
	Email     string        `json:"email"`
	Role      v1alpha1.Role `json:"role"`
	ExpiresAt time.Time     `json:"exp"`
}

func (s Session) expired() bool { return time.Now().After(s.ExpiresAt) }

// SessionManager signs and verifies session cookies with an HMAC over the
// JSON payload — no external session store, matching the single-replica,
// no-database scope (SPECS.md §1.2, §8.4).
type SessionManager struct {
	key []byte
}

func NewSessionManager(key []byte) *SessionManager {
	return &SessionManager{key: key}
}

func (m *SessionManager) sign(payload []byte) string {
	mac := hmac.New(sha256.New, m.key)
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Issue sets a signed session cookie for subject/email/role on w.
func (m *SessionManager) Issue(w http.ResponseWriter, subject, email string, role v1alpha1.Role) error {
	sess := Session{Subject: subject, Email: email, Role: role, ExpiresAt: time.Now().Add(sessionTTL)}
	payload, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	value := encoded + "." + m.sign(payload)

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})
	return nil
}

// Clear removes the session cookie (logout).
func (m *SessionManager) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

var ErrNoSession = errors.New("no valid session")

// FromRequest verifies and decodes the session cookie on r.
func (m *SessionManager) FromRequest(r *http.Request) (*Session, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, ErrNoSession
	}
	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return nil, ErrNoSession
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrNoSession
	}
	if !hmac.Equal([]byte(m.sign(payload)), []byte(parts[1])) {
		return nil, ErrNoSession
	}
	var sess Session
	if err := json.Unmarshal(payload, &sess); err != nil {
		return nil, ErrNoSession
	}
	if sess.expired() {
		return nil, ErrNoSession
	}
	return &sess, nil
}

type sessionContextKey struct{}

func withSession(ctx context.Context, sess *Session) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, sess)
}

// SessionFromContext returns the session set by RequireAuth, if any.
func SessionFromContext(ctx context.Context) (*Session, bool) {
	sess, ok := ctx.Value(sessionContextKey{}).(*Session)
	return sess, ok
}
