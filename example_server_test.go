// Run the rickover server.
//
// All of the project defaults are used. There is one authenticated user for
// basic auth, the user is "test" and the password is "hymanrickover". You will
// want to copy this binary and add your own authentication scheme.
package rickover

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/kevinburke/handlers"
	"github.com/kevinburke/rickover/config"
	"github.com/kevinburke/rickover/metrics"
	"github.com/kevinburke/rickover/models/db"
	"github.com/kevinburke/rickover/server"
	"github.com/kevinburke/rickover/setup"
)

var serverDbConns int

func init() {
	var err error
	serverDbConns, err = config.GetInt("PG_SERVER_POOL_SIZE")
	if err != nil {
		log.Printf("Error getting database pool size: %s. Defaulting to 10", err)
		serverDbConns = 10
	}

	// Change this user to a private value
	server.AddUser("test", "hymanrickover")
}

func Example_server() {
	if err := setup.DB(context.TODO(), db.DefaultConnection, serverDbConns); err != nil {
		log.Fatal(err)
	}

	go metrics.Run(context.TODO(), metrics.LibratoConfig{
		Namespace: "rickover.server",
		Source:    "web",
		Email:     "TODO@example.com",
	})

	go setup.MeasureActiveQueries(context.TODO(), 5*time.Second)

	log.Println("Listening on port 9090")
	log.Fatal(http.ListenAndServe(":9090", handlers.Log(server.DefaultServer)))
}
