package bothold_store

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	bh "github.com/timshannon/bolthold"
)

type TestDb struct {
	db       *bh.Store
	fileName string
}

// tempfile returns a temporary file path.
func tempfile() string {
	f, err := os.CreateTemp("", "bolthold_store_*")
	if err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
	if err := os.Remove(f.Name()); err != nil {
		panic(err)
	}
	return f.Name()
}

func NewTestDb() (*TestDb, error) {
	var db *bh.Store
	var err error

	fileName := tempfile()
	if db, err = bh.Open(fileName, 0600, nil); err != nil {
		return nil, err
	}

	return &TestDb{db, fileName}, nil
}

func (store *TestDb) Delete() {
	store.db.Close()
	os.Remove(store.fileName)

}

// TODO: this is ugly
func parseCookies(value string) map[string]*http.Cookie {
	m := map[string]*http.Cookie{}
	for _, c := range (&http.Request{Header: http.Header{"Cookie": {value}}}).Cookies() {
		m[c.Name] = c
	}
	return m
}

func testWithDb(t *testing.T, testFunc func(*testing.T, *TestDb)) {
	db, err := NewTestDb()
	if err != nil {
		t.Error(err)
		return
	}
	defer db.Delete()
	testFunc(t, db)
}

func testWithStore(t *testing.T, testFunc func(*testing.T, *Store)) {
	testWithDb(t, func(t *testing.T, db *TestDb) {
		sessionStore := NewSessionStore(db.db, []byte("secret"))
		testFunc(t, sessionStore)
	})
}

func req(handler http.HandlerFunc, sessionCookie *http.Cookie) *httptest.ResponseRecorder {
	req, _ := http.NewRequest("GET", "http://test", nil)
	if sessionCookie != nil {
		req.Header.Add("Cookie", fmt.Sprintf("%s=%s", sessionCookie.Name, sessionCookie.Value))
	}
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func match(t *testing.T, resp *httptest.ResponseRecorder, code int, body string) {
	if resp.Code != code {
		t.Errorf("Expected %v, actual %v", code, resp.Code)
	}
	// http.Error in countHandler adds a \n
	if strings.Trim(resp.Body.String(), "\n") != body {
		t.Errorf("Expected %v, actual %v", body, resp.Body)
	}
}

func findSession(db *bh.Store, id string) *BotholdSession {
	s := &BotholdSession{}
	err := db.FindOne(s, bh.Where("ID").Eq(id))
	if err != nil {
		return nil
	}
	return s
}

func makeCountHandler(name string, store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := store.Get(r, name)
		if err != nil {
			panic(err)
		}

		count, _ := session.Values["count"].(int)
		count++
		session.Values["count"] = count
		if err := store.Save(r, w, session); err != nil {
			panic(err)
		}
		// leak session ID so we can mess with it in the db
		w.Header().Add("X-Session", session.ID)
		http.Error(w, fmt.Sprintf("%d", count), http.StatusOK)
	}
}

func TestBasic(t *testing.T) {
	testWithStore(t, func(t *testing.T, store *Store) {
		countFn := makeCountHandler("session", store)
		r1 := req(countFn, nil)
		match(t, r1, 200, "1")
		r2 := req(countFn, parseCookies(r1.Header().Get("Set-Cookie"))["session"])
		match(t, r2, 200, "2")
	})
}

func TestExpire(t *testing.T) {
	testWithStore(t, func(t *testing.T, store *Store) {
		countFn := makeCountHandler("session", store)

		r1 := req(countFn, nil)
		match(t, r1, 200, "1")

		// test still in db but expired
		id := r1.Header().Get("X-Session")
		s := findSession(store.db, id)

		s.ExpiresAt = time.Now().Add(-40 * 24 * time.Hour)
		store.db.Update(s.ID, s)

		r2 := req(countFn, parseCookies(r1.Header().Get("Set-Cookie"))["session"])
		match(t, r2, 200, "1")

		store.Cleanup()

		if findSession(store.db, id) != nil {
			t.Error("Expected session to be deleted")
		}
	})
}

func TestBrokenCookie(t *testing.T) {
	testWithStore(t, func(t *testing.T, store *Store) {
		countFn := makeCountHandler("session", store)

		r1 := req(countFn, nil)
		match(t, r1, 200, "1")

		cookie := parseCookies(r1.Header().Get("Set-Cookie"))["session"]
		cookie.Value += "junk"
		r2 := req(countFn, cookie)
		match(t, r2, 200, "1")
	})
}

func TestMaxAgeNegative(t *testing.T) {
	testWithStore(t, func(t *testing.T, store *Store) {
		countFn := makeCountHandler("session", store)

		r1 := req(countFn, nil)
		match(t, r1, 200, "1")

		r2 := req(func(w http.ResponseWriter, r *http.Request) {
			session, err := store.Get(r, "session")
			if err != nil {
				panic(err)
			}

			session.Options.MaxAge = -1
			store.Save(r, w, session)

			http.Error(w, "", http.StatusOK)
		}, parseCookies(r1.Header().Get("Set-Cookie"))["session"])

		match(t, r2, 200, "")
		c := parseCookies(r2.Header().Get("Set-Cookie"))["session"]
		if c.Value != "" {
			t.Error("Expected empty Set-Cookie session header", c)
		}

		id := r1.Header().Get("X-Session")
		if s := findSession(store.db, id); s != nil {
			t.Error("Expected session to be deleted")
		}
	})
}

func TestMaxLength(t *testing.T) {
	testWithStore(t, func(t *testing.T, store *Store) {
		store.MaxLength(10)

		r1 := req(func(w http.ResponseWriter, r *http.Request) {
			session, err := store.Get(r, "session")
			if err != nil {
				panic(err)
			}

			session.Values["a"] = "aaaaaaaaaaaaaaaaaaaaaaaa"
			if err := store.Save(r, w, session); err == nil {
				t.Error("Expected too large error")
			}

			http.Error(w, "", http.StatusOK)
		}, nil)
		match(t, r1, 200, "")
	})
}

func TestMultiSessions(t *testing.T) {
	testWithStore(t, func(t *testing.T, store *Store) {
		countFn1 := makeCountHandler("session1", store)
		countFn2 := makeCountHandler("session2", store)

		r1 := req(countFn1, nil)
		match(t, r1, 200, "1")
		r2 := req(countFn2, nil)
		match(t, r2, 200, "1")

		r3 := req(countFn1, parseCookies(r1.Header().Get("Set-Cookie"))["session1"])
		match(t, r3, 200, "2")
		r4 := req(countFn2, parseCookies(r2.Header().Get("Set-Cookie"))["session2"])
		match(t, r4, 200, "2")
	})
}

func TestReuseSessionByName(t *testing.T) {
	testWithStore(t, func(t *testing.T, store *Store) {
		sessionName := "test-session"

		handler := func(w http.ResponseWriter, r *http.Request) {
			session, err := store.New(r, sessionName)
			if err != nil {
				panic(err)
			}
			session.ID = ""
			if err := store.Save(r, w, session); err != nil {
				panic(err)
			}
			http.Error(w, "", http.StatusOK)
		}

		r1 := req(handler, nil)
		match(t, r1, 200, "")
		r2 := req(handler, parseCookies(r1.Header().Get("Set-Cookie"))[sessionName])
		match(t, r2, 200, "")

		count, err := store.db.Count(&BotholdSession{}, nil)
		if err != nil {
			t.Error(err)
			return
		}
		if count != 1 {
			t.Error("An existing session with the same name should be reused")
			return
		}
	})
}

func TestPeriodicCleanup(t *testing.T) {
	testWithStore(t, func(t *testing.T, store *Store) {
		store.SessionOpts.MaxAge = 1
		countFn := makeCountHandler("session", store)

		quit := make(chan struct{})
		go store.PeriodicCleanup(200*time.Millisecond, quit)

		// test that cleanup i done at least twice

		r1 := req(countFn, nil)
		id1 := r1.Header().Get("X-Session")

		if findSession(store.db, id1) == nil {
			t.Error("Expected r1 session to exist")
		}

		time.Sleep(2 * time.Second)

		if findSession(store.db, id1) != nil {
			t.Error("Expected r1 session to be deleted")
		}

		r2 := req(countFn, nil)
		id2 := r2.Header().Get("X-Session")

		if findSession(store.db, id2) == nil {
			t.Error("Expected r2 session to exist")
		}

		time.Sleep(2 * time.Second)

		if findSession(store.db, id2) != nil {
			t.Error("Expected r2 session to be deleted")
		}

		close(quit)

		// test that cleanup has stopped

		r3 := req(countFn, nil)
		id3 := r3.Header().Get("X-Session")

		if findSession(store.db, id3) == nil {
			t.Error("Expected r3 session to exist")
		}

		time.Sleep(2 * time.Second)

		if findSession(store.db, id3) == nil {
			t.Error("Expected r3 session to exist")
		}
	})
}
