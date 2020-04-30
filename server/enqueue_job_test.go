package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kevinburke/rest"
	"github.com/kevinburke/rickover/test"
)

func newSSAServer() (*SharedSecretAuthorizer, http.Handler) {
	ssa := NewSharedSecretAuthorizer()
	return ssa, Get(Config{Auth: ssa})
}

var empty = json.RawMessage([]byte("{}"))

func Test401UnknownUser(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	ejr := &EnqueueJobRequest{
		Data: empty,
	}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(ejr)
	req, _ := http.NewRequest("PUT", "/v1/jobs/echo/job_6740b44e-13b9-475d-af06-979627e0e0d6", b)
	req.SetBasicAuth("unknown-user", "foobar")
	_, server := newSSAServer()
	server.ServeHTTP(w, req)
	test.AssertEquals(t, w.Code, http.StatusForbidden)
	var e rest.Error
	err := json.Unmarshal(w.Body.Bytes(), &e)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, e.Title, "Username or password are invalid. Please double check your credentials")
	test.AssertEquals(t, e.ID, "forbidden")
}

func Test401UnknownPassword(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	ejr := &EnqueueJobRequest{
		Data: empty,
	}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(ejr)
	ssa, server := newSSAServer()
	ssa.AddUser("401-unknown-password", "right_password")
	req, _ := http.NewRequest("PUT", "/v1/jobs/echo/job_6740b44e-13b9-475d-af06-979627e0e0d6", b)
	req.SetBasicAuth("401-unknown-password", "wrong_password")
	server.ServeHTTP(w, req)
	test.AssertEquals(t, w.Code, http.StatusForbidden)
	var e rest.Error
	err := json.Unmarshal(w.Body.Bytes(), &e)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, e.Title, "Incorrect password for user 401-unknown-password")
	test.AssertEquals(t, e.ID, "incorrect_password")
}

func Test400NoBody(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	ejr := &EnqueueJobRequest{
		Data: empty,
	}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(ejr)
	ssa, server := newSSAServer()
	ssa.AddUser("test", "password")
	req, _ := http.NewRequest("PUT", "/v1/jobs/echo/job_f17373a6-2cd7-4010-afba-eebc6dc6f9ab", nil)
	req.SetBasicAuth("test", "password")
	server.ServeHTTP(w, req)
	test.AssertEquals(t, w.Code, http.StatusBadRequest)
	var e rest.Error
	err := json.Unmarshal(w.Body.Bytes(), &e)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, e.Title, "Missing required field: data")
	test.AssertEquals(t, e.ID, "missing_parameter")
	test.AssertEquals(t, e.Instance, "/v1/jobs/echo/job_f17373a6-2cd7-4010-afba-eebc6dc6f9ab")
}

func Test400EmptyBody(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	var v interface{}
	err := json.Unmarshal([]byte("{}"), &v)
	test.AssertNotError(t, err, "")
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(v)
	ssa, server := newSSAServer()
	ssa.AddUser("test", "password")
	req, _ := http.NewRequest("PUT", "/v1/jobs/echo/job_f17373a6-2cd7-4010-afba-eebc6dc6f9ab", b)
	req.SetBasicAuth("test", "password")
	server.ServeHTTP(w, req)
	test.AssertEquals(t, w.Code, http.StatusBadRequest)
	var e rest.Error
	err = json.Unmarshal(w.Body.Bytes(), &e)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, e.Title, "Missing required field: data")
	test.AssertEquals(t, e.ID, "missing_parameter")
	test.AssertEquals(t, e.Instance, "/v1/jobs/echo/job_f17373a6-2cd7-4010-afba-eebc6dc6f9ab")
}

func Test400InvalidUUID(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	ejr := &EnqueueJobRequest{
		Data: empty,
	}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(ejr)
	req, _ := http.NewRequest("PUT", "/v1/jobs/echo/job_zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", b)
	req.SetBasicAuth("test", "password")
	Get(u).ServeHTTP(w, req)
	test.AssertEquals(t, w.Code, http.StatusBadRequest)
	var e rest.Error
	err := json.Unmarshal(w.Body.Bytes(), &e)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, e.Title, "incorrect UUID format zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz")
	test.AssertEquals(t, e.ID, "invalid_uuid")
}

func Test400WrongPrefix(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	ejr := &EnqueueJobRequest{
		Data: empty,
	}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(ejr)
	req, _ := http.NewRequest("PUT", "/v1/jobs/echo/usr_6740b44e-13b9-475d-af06-979627e0e0d6", b)
	req.SetBasicAuth("test", "password")
	Get(u).ServeHTTP(w, req)
	test.AssertEquals(t, w.Code, http.StatusBadRequest)
}

func Test413TooLargeJSON(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	// 4 bytes per record - the value and the quotes around it.
	var bigarr [100 * 256]string
	for i := range bigarr {
		bigarr[i] = "a"
	}
	bits, _ := json.Marshal(bigarr)
	ejr := &EnqueueJobRequest{
		Data: json.RawMessage(bits),
	}
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(ejr)
	test.Assert(t, len(b.Bytes()) > 100*1024, fmt.Sprintf("%d", len(b.Bytes())))
	req, _ := http.NewRequest("PUT", "/v1/jobs/echo/job_6740b44e-13b9-475d-af06-979627e0e0d6", b)
	req.SetBasicAuth("test", "password")
	Get(u).ServeHTTP(w, req)
	test.AssertEquals(t, w.Code, http.StatusRequestEntityTooLarge)
	var e rest.Error
	err := json.Unmarshal(w.Body.Bytes(), &e)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, e.Title, "Data parameter is too large (100KB max)")
	test.AssertEquals(t, e.ID, "entity_too_large")
}
