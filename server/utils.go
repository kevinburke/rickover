package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/kevinburke/go-types"
	"github.com/kevinburke/rest"
	"github.com/kevinburke/rest/resterror"
	"github.com/kevinburke/rickover/models/queued_jobs"
)

// getId validates that the provided ID is valid, and the prefix matches the
// expected prefix. Returns the correct ID, and a boolean describing whether
// the helper has written a response.
func getId(w http.ResponseWriter, r *http.Request, idStr string) (types.PrefixUUID, bool) {
	id, err := types.NewPrefixUUID(idStr)
	if err != nil {
		msg := strings.Replace(err.Error(), "types: ", "", 1)
		msg = strings.Replace(msg, "uuid: ", "", 1)
		rest.BadRequest(w, r, &resterror.Error{
			ID:    "invalid_uuid",
			Title: msg,
		})
		return id, true
	}
	if id.Prefix == "" {
		id.Prefix = queued_jobs.Prefix
	}
	if id.Prefix != queued_jobs.Prefix {
		rest.BadRequest(w, r, &resterror.Error{
			ID:    "invalid_prefix",
			Title: fmt.Sprintf("Please use %q for the uuid prefix, not %s", queued_jobs.Prefix, id.Prefix),
		})
		return id, true
	}
	return id, false
}
