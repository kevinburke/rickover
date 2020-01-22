// Run the rickover server.
//
// All of the project defaults are used. There is one authenticated user for
// basic auth, the user is "test" and the password is "hymanrickover". You will
// want to copy this binary and add your own authentication scheme.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	metrics "github.com/kevinburke/go-simple-metrics"
	"github.com/kevinburke/handlers"
	"github.com/kevinburke/rickover/config"
	"github.com/kevinburke/rickover/models/db"
	"github.com/kevinburke/rickover/server"
	"github.com/kevinburke/rickover/setup"
)

func configure(ctx context.Context) (http.Handler, error) {
	dbConns, err := config.GetInt("PG_SERVER_POOL_SIZE")
	if err != nil {
		log.Printf("Error getting database pool size: %s. Defaulting to 10", err)
		dbConns = 10
	}

	if err = setup.DB(ctx, db.DefaultConnection, dbConns); err != nil {
		return nil, err
	}

	metrics.Namespace = "rickover.server"
	metrics.Start("web", os.Getenv("LIBRATO_EMAIL_ACCOUNT"))

	go setup.MeasureActiveQueries(ctx, 5*time.Second)

	// If you run this in production, change this user.
	server.AddUser("test", "hymanrickover")
	return server.Get(server.DefaultAuthorizer), nil
}

func main() {
	flag.Parse()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := configure(ctx)
	if err != nil {
		log.Fatal(err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "9090"
	}
	log.Printf("Listening on port %s\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), handlers.Log(s)))
}
