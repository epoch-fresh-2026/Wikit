// Package identity owns accounts, credential lifecycle and authentication.
package identity

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/epoch-fresh-2026/Wikit/internal/core/fault"
)

type Account struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
}
type Record struct {
	Account
	PasswordHash string
	AuthVersion  int64
}
type Token struct {
	UserID, Digest string
	Generation     int64
	CreatedAt      time.Time
}
type Session struct {
	Digest, UserID, CSRFDigest string
	AuthVersion                int64
	ExpiresAt                  time.Time
}
type Credentials struct {
	Session, Bearer, CSRF        string
	Write, OriginOK, SessionOnly bool
}
type Actor struct {
	Record
	Method     string
	Generation int64
}

// Principal is safe authentication metadata for a transport's session binding.
type Principal struct {
	ID                      string
	AuthVersion, Generation int64
}

func (s *Service) Authenticate(ctx context.Context, c Credentials) (Principal, error) {
	var out Principal
	err := s.repo.WithinIdentity(ctx, func(tx Tx) error {
		a, err := Require(ctx, tx, c)
		if err == nil {
			out = Principal{a.ID, a.AuthVersion, a.Generation}
		}
		return err
	})
	if err != nil {
		return Principal{}, err
	}
	return out, nil
}

type Audit struct {
	ID        string    `json:"id"`
	ActorID   *string   `json:"actor_id"`
	Action    string    `json:"action"`
	TargetID  *string   `json:"target_id"`
	RequestID string    `json:"request_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Authentication is the minimum transaction view needed to resolve credentials.
type Authentication interface {
	User(context.Context, string) (Record, error)
	FindToken(context.Context, string) (Token, error)
	FindSession(context.Context, string) (Session, error)
}
type Tx interface {
	Authentication
	UserByName(context.Context, string) (Record, error)
	InsertUser(context.Context, Record) error
	SaveUser(context.Context, Record) error
	Token(context.Context, string) (Token, error)
	SaveToken(context.Context, Token) error
	SaveSession(context.Context, Session) error
	DeleteSession(context.Context, string) error
	DeleteSessions(context.Context, string) error
	Audit(context.Context, Actor, string, string, string) error
}
type Repository interface {
	WithinIdentity(context.Context, func(Tx) error) error
	// Allow commits counters independently, including failed login attempts.
	Allow(context.Context, string, string) error
}
type Passwords interface {
	Hash(string) (string, error)
	Verify(string, string) bool
}
type Secrets interface {
	ID() (string, error)
	Secret() (string, error)
}
type Options struct {
	Registration bool
	SessionTTL   time.Duration
}
type Service struct {
	repo      Repository
	passwords Passwords
	secrets   Secrets
	options   Options
}

func New(repo Repository, passwords Passwords, secrets Secrets, options Options) *Service {
	return &Service{repo, passwords, secrets, options}
}

func Digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func ValidID(id string) bool {
	return regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`).MatchString(strings.ToLower(id))
}
func ValidPassword(value string) bool {
	n := utf8.RuneCountInString(value)
	return utf8.ValidString(value) && n >= 12 && n <= 128
}
func validName(value string) bool {
	return regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`).MatchString(value)
}

// Resolve re-reads current account state on every invocation. Consumers must use
// the same authorization transaction for the protected action.
func Resolve(ctx context.Context, tx Authentication, c Credentials) (Actor, error) {
	var a, b Actor
	if c.Session != "" {
		s, err := tx.FindSession(ctx, Digest(c.Session))
		if err != nil {
			return Actor{}, err
		}
		if !s.ExpiresAt.After(time.Now()) {
			return Actor{}, fault.Unauthenticated
		}
		u, err := tx.User(ctx, s.UserID)
		if err != nil {
			return Actor{}, err
		}
		if u.Status != "active" || u.AuthVersion != s.AuthVersion {
			return Actor{}, fault.Unauthenticated
		}
		if c.Write && (!c.OriginOK || subtle.ConstantTimeCompare([]byte(s.CSRFDigest), []byte(Digest(c.CSRF))) != 1) {
			return Actor{}, fault.Forbidden
		}
		a = Actor{Record: u, Method: "session"}
	}
	if c.Bearer != "" {
		t, err := tx.FindToken(ctx, Digest(c.Bearer))
		if err != nil {
			return Actor{}, err
		}
		u, err := tx.User(ctx, t.UserID)
		if err != nil {
			return Actor{}, err
		}
		if u.Status != "active" {
			return Actor{}, fault.Unauthenticated
		}
		b = Actor{Record: u, Method: "bearer", Generation: t.Generation}
	}
	if a.ID != "" && b.ID != "" && a.ID != b.ID {
		return Actor{}, fault.BadRequest
	}
	if c.SessionOnly && a.ID == "" {
		return Actor{}, fault.Unauthenticated
	}
	if a.ID != "" {
		return a, nil
	}
	if b.ID != "" {
		return b, nil
	}
	return Actor{}, nil
}
func Require(ctx context.Context, tx Authentication, c Credentials) (Actor, error) {
	a, err := Resolve(ctx, tx, c)
	if err != nil {
		return Actor{}, err
	}
	if a.ID == "" {
		return Actor{}, fault.Unauthenticated
	}
	return a, nil
}
func (s *Service) Current(ctx context.Context, c Credentials) (Account, error) {
	var out Account
	err := s.repo.WithinIdentity(ctx, func(tx Tx) error { a, err := Require(ctx, tx, c); out = a.Account; return err })
	return out, err
}
func (s *Service) create(ctx context.Context, username, password, role, requestID string) (Account, string, error) {
	if !validName(username) || !ValidPassword(password) {
		return Account{}, "", fault.Invalid
	}
	hash, err := s.passwords.Hash(password)
	if err != nil {
		return Account{}, "", err
	}
	id, err := s.secrets.ID()
	if err != nil {
		return Account{}, "", err
	}
	plain, err := s.secrets.Secret()
	if err != nil {
		return Account{}, "", err
	}
	u := Record{Account: Account{id, username, username, role, "active"}, PasswordHash: hash, AuthVersion: 1}
	err = s.repo.WithinIdentity(ctx, func(tx Tx) error {
		if err := tx.InsertUser(ctx, u); err != nil {
			return err
		}
		if err := tx.SaveToken(ctx, Token{id, Digest(plain), 1, time.Now().UTC()}); err != nil {
			return err
		}
		method := "anonymous"
		if role == "admin" {
			method = "local"
		}
		return tx.Audit(ctx, Actor{Record: u, Method: method}, "account.create", id, requestID)
	})
	return u.Account, plain, err
}
func (s *Service) Register(ctx context.Context, username, password, ip, requestID string, originOK bool) (Account, string, error) {
	if !originOK {
		return Account{}, "", fault.Forbidden
	}
	if !s.options.Registration {
		return Account{}, "", fault.Forbidden
	}
	if err := s.repo.Allow(ctx, "register", ip+"\x00"+strings.ToLower(username)); err != nil {
		return Account{}, "", err
	}
	return s.create(ctx, username, password, "member", requestID)
}
func (s *Service) CreateAdmin(ctx context.Context, username, password, requestID string) (Account, error) {
	a, _, err := s.create(ctx, username, password, "admin", requestID)
	return a, err
}

type Login struct {
	Account       Account
	Session, CSRF string
}

func (s *Service) Login(ctx context.Context, username, password, ip, requestID string, originOK bool) (Login, error) {
	if !originOK {
		return Login{}, fault.Forbidden
	}
	if len(username) > 32 || len(password) > 512 {
		return Login{}, fault.Invalid
	}
	if err := s.repo.Allow(ctx, "login", ip+"\x00"+strings.ToLower(username)); err != nil {
		return Login{}, err
	}
	var out Login
	err := s.repo.WithinIdentity(ctx, func(tx Tx) error {
		u, err := tx.UserByName(ctx, strings.ToLower(username))
		if err != nil {
			if err == fault.NotFound {
				s.passwords.Verify(password, "")
				return fault.Unauthenticated
			}
			return err
		}
		if !s.passwords.Verify(password, u.PasswordHash) || u.Status != "active" {
			return fault.Unauthenticated
		}
		plain, err := s.secrets.Secret()
		if err != nil {
			return err
		}
		csrf, err := s.secrets.Secret()
		if err != nil {
			return err
		}
		if err := tx.SaveSession(ctx, Session{Digest(plain), u.ID, Digest(csrf), u.AuthVersion, time.Now().Add(s.options.SessionTTL)}); err != nil {
			return err
		}
		out = Login{u.Account, plain, csrf}
		return tx.Audit(ctx, Actor{Record: u, Method: "session"}, "session.create", u.ID, requestID)
	})
	return out, err
}
func (s *Service) Logout(ctx context.Context, c Credentials, requestID string) error {
	c.Write = true
	c.SessionOnly = true
	if !c.OriginOK {
		return fault.Forbidden
	}
	return s.repo.WithinIdentity(ctx, func(tx Tx) error {
		if c.Session == "" {
			return nil
		}
		a, err := Require(ctx, tx, c)
		if err == fault.NotFound || err == fault.Unauthenticated {
			return nil
		}
		if err != nil {
			return err
		}
		if err := tx.DeleteSession(ctx, Digest(c.Session)); err != nil {
			return err
		}
		return tx.Audit(ctx, a, "session.delete", a.ID, requestID)
	})
}
func (s *Service) TokenStatus(ctx context.Context, c Credentials) (Token, error) {
	var out Token
	c.SessionOnly = true
	err := s.repo.WithinIdentity(ctx, func(tx Tx) error {
		a, err := Require(ctx, tx, c)
		if err != nil {
			return err
		}
		out, err = tx.Token(ctx, a.ID)
		return err
	})
	return out, err
}
func (s *Service) ChangeSecret(ctx context.Context, c Credentials, password, newPassword, operation, requestID string) (string, Token, error) {
	c.SessionOnly = true
	c.Write = true
	var plain string
	var token Token
	var userID string
	err := s.repo.WithinIdentity(ctx, func(tx Tx) error { a, err := Require(ctx, tx, c); userID = a.ID; return err })
	if err != nil {
		return "", token, err
	}
	if err := s.repo.Allow(ctx, "sensitive", userID+"\x00"+userID); err != nil {
		return "", token, err
	}
	// Re-resolve in the mutation transaction after throttling. Never hold a pool
	// connection while acquiring a second one for independently committed counters.
	err = s.repo.WithinIdentity(ctx, func(tx Tx) error {
		a, err := Require(ctx, tx, c)
		if err != nil {
			return err
		}
		if !s.passwords.Verify(password, a.PasswordHash) {
			return fault.Unauthenticated
		}
		if operation == "password" {
			if !ValidPassword(newPassword) {
				return fault.Invalid
			}
			hash, err := s.passwords.Hash(newPassword)
			if err != nil {
				return err
			}
			a.PasswordHash = hash
			a.AuthVersion++
			if err := tx.SaveUser(ctx, a.Record); err != nil {
				return err
			}
			if err := tx.DeleteSessions(ctx, a.ID); err != nil {
				return err
			}
		} else {
			token, err = tx.Token(ctx, a.ID)
			if err != nil {
				return err
			}
			if operation == "rotate" {
				plain, err = s.secrets.Secret()
				if err != nil {
					return err
				}
				token.Digest = Digest(plain)
				token.Generation++
				token.CreatedAt = time.Now().UTC()
			} else if operation == "revoke" {
				token.Digest = ""
			} else {
				return fault.BadRequest
			}
			if err := tx.SaveToken(ctx, token); err != nil {
				return err
			}
		}
		return tx.Audit(ctx, a, "credential."+operation, a.ID, requestID)
	})
	return plain, token, err
}
