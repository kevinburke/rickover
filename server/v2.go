package server

import (
	"net/http"
	"regexp"

	"github.com/kevinburke/rest"
	"github.com/kevinburke/rickover/dbtohttp"
	"github.com/kevinburke/rickover/newmodels"
)

func V2(db *newmodels.Queries) http.Handler {
	h := new(RegexpHandler)
	// auth is handled external to this function
	h.Handler(regexp.MustCompile(`^/v2/job-types$`), []string{"GET"}, v2GetJobTypes(db))
	return h
}

// GET /v2/job-types
//
// List all job types.
func v2GetJobTypes(db *newmodels.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobs, err := db.GetAllJobs(r.Context())
		if err != nil {
			rest.ServerError(w, r, err)
			return
		}
		respond(Logger, w, r, dbtohttp.Jobs(jobs))
	}
}
