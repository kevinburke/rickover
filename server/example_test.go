package server

import (
	"net/http"

	"github.com/kevinburke/rest/resterror"
)

type auther struct{}

func (a *auther) Authorize(userId, token string) *resterror.Error {
	// Implement your auth scheme here.
	return nil
}

func Example() {
	// Get all server routes using your authorization handler, then listen on
	// port 9090
	handler := Get(Config{Auth: &auther{}})
	http.ListenAndServe(":9090", handler)
}
