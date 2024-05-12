package scylladb

import (
	"context"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/stretchr/testify/require"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestScyllaDB(t *testing.T) {
	r := require.New(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	app, err := New(ctx)
	r.NoError(err)

	defer app.Close(ctx)

	err = app.CreateKeyspace("blah")
	r.NoError(err)

	sc, err := app.ClusterConfig("blah")
	r.NoError(err)

	session, err := sc.CreateSession()
	r.NoError(err)

	err = session.Query(`CREATE TABLE testtable (id int, item text, primary key(id));`).Exec()
	r.NoError(err)

	err = session.Query(`INSERT INTO testtable(id, item) VALUES (?, ?);`, 1, "test").Exec()
	r.NoError(err)

	var (
		id   int
		item string
	)
	err = session.Query("SELECT id,item FROM testtable WHERE id = ?", 1).Scan(&id, &item)
	r.NoError(err)
	r.Equal(1, id)
	r.Equal("test", item)

	session.Close()

	err = app.DropKeyspace("blah")
	r.NoError(err)
}
