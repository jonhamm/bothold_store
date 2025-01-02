#### BOTHOLD backend for gorilla sessions


#### Documentation

for 
```go

import (
    	bh "github.com/timshannon/bolthold"
    	bhs "github.com/jonhamm/bolthold-store"
)

// initialize and setup cleanup
store := bhs.New(bh.Open(...), []byte("secret"))
// db cleanup every hour
// close quit channel to stop cleanup
quit := make(chan struct{})
go store.PeriodicCleanup(1*time.Hour, quit)
```

```go
// in HTTP handler
func handlerFunc(w http.ResponseWriter, r *http.Request) {
  session, err := store.Get(r, "session")
  session.Values["user_id"] = 123
  store.Save(r, w, session)
  http.Error(w, "", http.StatusOK)
}
```

For more details see [bothold_store documentation](https://pkg.go.dev/github.com/wader/bothold_store?tab=doc).

#### Testing

Just sqlite3 tests:

    go test

All databases using docker:

    ./test

If docker is not local (docker-machine etc):

    DOCKER_IP=$(docker-machine ip dev) ./test

#### License

bothold_store is licensed under the MIT license. See [LICENSE](LICENSE) for the full license text.
