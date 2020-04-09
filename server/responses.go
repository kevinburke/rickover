package server

import (
	"encoding/json"
	"net/http"

	log "github.com/inconshreveable/log15"
	"github.com/kevinburke/rest"
)

func respondCode(v log.Logger, w http.ResponseWriter, r *http.Request, code int, msg interface{}) {
	// TODO content negotiation
	data, err := json.Marshal(msg)
	if err != nil {
		rest.ServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	w.Write(data)
}

func respond(v log.Logger, w http.ResponseWriter, r *http.Request, msg interface{}) {
	respondCode(v, w, r, http.StatusOK, msg)
}

func created(v log.Logger, w http.ResponseWriter, r *http.Request, msg interface{}) {
	respondCode(v, w, r, http.StatusCreated, msg)
}

func accepted(v log.Logger, w http.ResponseWriter, r *http.Request, msg interface{}) {
	respondCode(v, w, r, http.StatusAccepted, msg)
}
