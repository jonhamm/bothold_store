package bothold_store

import (
	"time"

	"github.com/gin-contrib/sessions"
	gs "github.com/gorilla/sessions"
	bh "github.com/timshannon/bolthold"
)

type Store interface {
	sessions.Store
}

func NewStore(db *bh.Store, expiredSessionCleanup bool, keyPairs ...[]byte) Store {
	sessionStore := NewSessionStore(db, keyPairs...)
	if expiredSessionCleanup {
		quit := make(chan struct{})
		go sessionStore.PeriodicCleanup(1*time.Hour, quit)
	}
	return &store{sessionStore}
}

type store struct {
	*DB
}

func (s *store) Options(options sessions.Options) {
	s.DB.SessionOpts = options.ToGorillaOptions()
}

func (s *store) SessionOptions() *sessions.Options {
	return fromGorillaOptions(s.DB.SessionOpts)
}

func fromGorillaOptions(options *gs.Options) *sessions.Options {
	return &sessions.Options{
		Path:     options.Path,
		Domain:   options.Domain,
		MaxAge:   options.MaxAge,
		Secure:   options.Secure,
		HttpOnly: options.HttpOnly,
		SameSite: options.SameSite,
	}
}
