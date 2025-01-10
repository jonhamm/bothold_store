/*
Package bothold_store is a BotHold backend for gorilla sessions

Simplest form:

	    import (
			bh "github.com/timshannon/bolthold"
		    bhs "github.com/jonhamm/bolthold_store"
		)

		func main() {
			. . . .
			var db *bothold.Store;
			var store *bhs.Store
			var err error

			if db,err = bothold.Open(...); err != nil {
				panic(err)
			}
			defer() db.Close()

			if store,err := bhs.New(db, []byte("secret-hash-key")); err != nil {
				panic(err)
			}

All options:

	store,err = bhs.NewSessionStore(
	    db,
		[]byte("secret-hash-key"),       // 32 or 64 bytes recommended, required
		[]byte("secret-encryption-key")) // nil, 16, 24 or 32 bytes, optional
	if err != nil {
		panic(err)
	}
	// some more settings, see sessions.Options
	store.SessionOpts.Secure = true
	store.SessionOpts.HttpOnly = true
	store.SessionOpts.MaxAge = 60 * 60 * 24 * 60

If you want periodic cleanup of expired sessions:

	quit := make(chan struct{})
	go store.PeriodicCleanup(1*time.Hour, quit)

For more information about the keys see https://github.com/gorilla/securecookie

For API to use in HTTP handlers see https://github.com/gorilla/sessions
*/
package bothold_store

import (
	"encoding/base32"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	bh "github.com/timshannon/bolthold"
)

const sessionIDLen = 32

// const defaultTableName = "sessions"
const defaultMaxAge = 60 * 60 * 24 * 30 // 30 days
const defaultPath = "/"

// DB represent a bothold_store
type DB struct {
	db          *bh.Store
	Codecs      []securecookie.Codec
	SessionOpts *sessions.Options
}

type BotholdSession struct {
	ID        string `botholdKey:"ID"`
	Data      string
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresAt time.Time `botholdIndex:"ExpiresAt"`
}

// / NewSessionStore creates a new bothold_store session with options
func NewSessionStore(db *bh.Store, keyPairs ...[]byte) *DB {
	st := &DB{
		db:     db,
		Codecs: securecookie.CodecsFromPairs(keyPairs...),
		SessionOpts: &sessions.Options{
			Path:   defaultPath,
			MaxAge: defaultMaxAge,
		},
	}
	//	if st.opts.TableName == "" {
	//		st.opts.TableName = defaultTableName
	//	}

	return st
}

// Get returns a session for the given name after adding it to the registry.
func (st *DB) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(st, name)
}

// New creates a session with name without adding it to the registry.
func (st *DB) New(r *http.Request, name string) (*sessions.Session, error) {
	session := sessions.NewSession(st, name)
	session.Options = st.SessionOpts
	session.IsNew = true

	st.MaxAge(st.SessionOpts.MaxAge)

	// try fetch from db if there is a cookie
	s := st.getSessionFromCookie(r, session.Name())
	if s != nil {
		if err := securecookie.DecodeMulti(session.Name(), s.Data, &session.Values, st.Codecs...); err != nil {
			return session, nil
		}
		session.ID = s.ID
		session.IsNew = false
	}

	return session, nil
}

// Save session and set cookie header
func (st *DB) Save(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	s := st.getSessionFromCookie(r, session.Name())

	// delete if max age is < 0
	if session.Options.MaxAge < 0 {
		if s != nil {
			if err := st.db.Delete(s.ID, s); err != nil {
				return err
			}
		}
		http.SetCookie(w, sessions.NewCookie(session.Name(), "", session.Options))
		return nil
	}

	data, err := securecookie.EncodeMulti(session.Name(), session.Values, st.Codecs...)
	if err != nil {
		return err
	}
	now := time.Now()
	expire := now.Add(time.Second * time.Duration(session.Options.MaxAge))

	if s == nil {
		// generate random session ID key suitable for storage in the db
		session.ID = strings.TrimRight(
			base32.StdEncoding.EncodeToString(
				securecookie.GenerateRandomKey(sessionIDLen)), "=")
		s = &BotholdSession{
			ID:        session.ID,
			Data:      data,
			CreatedAt: now,
			UpdatedAt: now,
			ExpiresAt: expire,
		}
		if err := st.db.Insert(s.ID, s); err != nil {
			return err
		}
	} else {
		s.Data = data
		s.UpdatedAt = now
		s.ExpiresAt = expire
		if err := st.db.Update(s.ID, s); err != nil {
			return err
		}
	}

	// set session id cookie
	id, err := securecookie.EncodeMulti(session.Name(), s.ID, st.Codecs...)
	if err != nil {
		return err
	}
	http.SetCookie(w, sessions.NewCookie(session.Name(), id, session.Options))

	return nil
}

// getSessionFromCookie looks for an existing BotholdSession from a session ID stored inside a cookie
func (st *DB) getSessionFromCookie(r *http.Request, name string) *BotholdSession {
	if cookie, err := r.Cookie(name); err == nil {
		sessionID := ""
		if err := securecookie.DecodeMulti(name, cookie.Value, &sessionID, st.Codecs...); err != nil {
			return nil
		}
		s := &BotholdSession{}
		err := st.db.FindOne(s, bh.Where("ID").Eq(sessionID).And("ExpiresAt").Gt(time.Now()))
		if err != nil {
			return nil
		}
		return s
	}
	return nil
}

// MaxAge sets the maximum age for the store and the underlying cookie
// implementation. Individual sessions can be deleted by setting
// Options.MaxAge = -1 for that session.
func (st *DB) MaxAge(age int) {
	st.SessionOpts.MaxAge = age
	for _, codec := range st.Codecs {
		if sc, ok := codec.(*securecookie.SecureCookie); ok {
			sc.MaxAge(age)
		}
	}
}

// MaxLength restricts the maximum length of new sessions to l.
// If l is 0 there is no limit to the size of a session, use with caution.
// The default is 4096 (default for securecookie)
func (st *DB) MaxLength(l int) {
	for _, c := range st.Codecs {
		if codec, ok := c.(*securecookie.SecureCookie); ok {
			codec.MaxLength(l)
		}
	}
}

// Cleanup deletes expired sessions
func (st *DB) Cleanup() {
	st.db.DeleteMatching(&BotholdSession{}, bh.Where("ExpiresAt").Le(time.Now()))
}

// PeriodicCleanup runs Cleanup every interval. Close quit channel to stop.
func (st *DB) PeriodicCleanup(interval time.Duration, quit <-chan struct{}) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			st.Cleanup()
		case <-quit:
			return
		}
	}
}
