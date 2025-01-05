package bothold_store

import (
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/tester"
)

var newStore = func(_ *testing.T) sessions.Store {
	db, err := NewTestDb()
	if err != nil {
		panic(err)
	}
	return NewStore(db.db, false, []byte("secret"))
}

func TestBothold_SessionGetSet(t *testing.T) {
	tester.GetSet(t, newStore)
}

func TestBothold_SessionDeleteKey(t *testing.T) {
	tester.DeleteKey(t, newStore)
}

func TestBothold_SessionFlashes(t *testing.T) {
	tester.Flashes(t, newStore)
}

func TestBothold_SessionClear(t *testing.T) {
	tester.Clear(t, newStore)
}

func TestBothold_SessionOptions(t *testing.T) {
	tester.Options(t, newStore)
}

func TestBothold_SessionMany(t *testing.T) {
	tester.Many(t, newStore)
}
