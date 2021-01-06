package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kevinburke/rest/resterror"
	"github.com/kevinburke/rickover/test"
)

func TestNoBody400(t *testing.T) {
	t.Parallel()
	req, _ := http.NewRequest("POST", "/v1/jobs/echo/job_f17373a6-2cd7-4010-afba-eebc6dc6f9ab", nil)
	req.SetBasicAuth("test", "password")
	w := httptest.NewRecorder()
	Get(u).ServeHTTP(w, req)
	test.AssertEquals(t, w.Code, http.StatusBadRequest)
	var err resterror.Error
	e := json.Unmarshal(w.Body.Bytes(), &err)
	test.AssertNotError(t, e, "unmarshaling body")
	test.AssertEquals(t, err.ID, "missing_parameter")
}
