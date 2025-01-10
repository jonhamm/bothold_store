package bothold_store

import (
	"time"

	"github.com/gin-contrib/sessions"
	bh "github.com/timshannon/bolthold"
)

type Store interface {
	sessions.Store
}

func NewStore(db *bh.Store, expiredSessionCleanup bool, keyPairs ...[]byte) Store {
	sessionStore := NewSessionStore(db, keyPairs...)
	sessionStore.SessionOpts.HttpOnly = true
	sessionStore.SessionOpts.Secure = true
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
