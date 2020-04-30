// Package server provides an HTTP interface for the job queue/scheduler.
package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/http/pprof"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kevinburke/go-types"
	"github.com/kevinburke/handlers"
	"github.com/kevinburke/rest"
	"github.com/kevinburke/rickover/config"
	"github.com/kevinburke/rickover/metrics"
	"github.com/kevinburke/rickover/models/archived_jobs"
	"github.com/kevinburke/rickover/models/db"
	"github.com/kevinburke/rickover/models/jobs"
	"github.com/kevinburke/rickover/models/queued_jobs"
	"github.com/kevinburke/rickover/newmodels"
	"github.com/kevinburke/rickover/setup"
	"github.com/lib/pq"
)

// TODO(burke) use http.LimitedBytesReader.

// The maximum data size that can be sent in the body of a HTTP request.
const MAX_ENQUEUE_DATA_SIZE = 100 * 1024

var disallowUnencryptedRequests = true

// DefaultServer serves every route using the DefaultAuthorizer for
// authentication.
var DefaultServer http.Handler

// POST /v1/jobs(/:name)/:id/replay
var replayRoute = regexp.MustCompile(`^/v1/jobs(/(?P<JobName>[^\s\/]+))?/(?P<id>job_[^\s\/]{20,40})/replay$`)

// GET /v1/jobs/job_123
//
// Must go before the getJobTypeRoute
var getJobRoute = regexp.MustCompile(`^/v1/jobs/(?P<id>job_[^\s\/]+)$`)

// GET/POST /v1/jobs
var jobsRoute = regexp.MustCompile("^/v1/jobs$")

// GET/POST/PUT /v1/jobs/:name/:id
var jobIdRoute = regexp.MustCompile(`^/v1/jobs/(?P<JobName>[^\s\/]+)/(?P<id>(job_)?[^\s\/]{20,40}|random_id)$`)

// GET /v1/jobs/:job-name
var getJobTypeRoute = regexp.MustCompile(`^/v1/jobs/(?P<JobName>[^\s\/]+)$`)

// GET /v1/archived-jobs
var archivedJobsRoute = regexp.MustCompile("^/v1/archived-jobs$")

type Config struct {
	// Authorizer to use. If nil, DefaultAuthorizer is used.
	Auth Authorizer
	// Database connector, for example db.DatabaseURLConnector. If nil,
	// db.DefaultConnection is used.
	Connector db.Connector
	// Number of open connections to the database
	NumConns int

	// Enqueueing a job with name "meta.shutdown" will shutdown the dequeuer (so
	// it can be restarted with a job type added or removed).
	//
	// Enable this flag if you have long running jobs that could be interfered
	// with if the dequeuer restarted.
	DisableMetaShutdown bool
}

// New initializes the database connection and returns a http.Handler that can
// run the server.
func New(ctx context.Context, cfg Config) (http.Handler, error) {
	if cfg.Auth == nil {
		cfg.Auth = DefaultAuthorizer
	}
	if cfg.Connector == nil {
		cfg.Connector = db.DefaultConnection
	}
	if err := setup.DB(ctx, cfg.Connector, cfg.NumConns); err != nil {
		return nil, err
	}
	s := Get(cfg)
	return s, nil
}

// Get returns a http.Handler with all routes initialized using the given
// Authorizer.
func Get(c Config) http.Handler {
	a := c.Auth
	if a == nil {
		panic("server: cannot call Get() with nil Authorizer")
	}
	h := new(RegexpHandler)

	useMetaShutdown := !c.DisableMetaShutdown
	h.Handler(jobsRoute, []string{"POST"}, authHandler(createJob(useMetaShutdown), a))
	h.Handler(getJobRoute, []string{"GET"}, authHandler(handleJobRoute(useMetaShutdown), a))
	h.Handler(getJobTypeRoute, []string{"GET"}, authHandler(getJobType(), a))

	h.Handler(replayRoute, []string{"POST"}, authHandler(replayHandler(), a))

	h.Handler(jobIdRoute, []string{"GET", "POST", "PUT"}, authHandler(handleJobRoute(useMetaShutdown), a))
	h.Handler(archivedJobsRoute, []string{"GET"}, authHandler(listArchivedJobs(), a))

	h.Handler(regexp.MustCompile("^/debug/pprof$"), []string{"GET"}, authHandler(http.HandlerFunc(pprof.Index), a))
	h.Handler(regexp.MustCompile("^/debug/pprof/cmdline$"), []string{"GET"}, authHandler(http.HandlerFunc(pprof.Cmdline), a))
	h.Handler(regexp.MustCompile("^/debug/pprof/profile$"), []string{"GET"}, authHandler(http.HandlerFunc(pprof.Profile), a))
	h.Handler(regexp.MustCompile("^/debug/pprof/symbol$"), []string{"GET"}, authHandler(http.HandlerFunc(pprof.Symbol), a))
	h.Handler(regexp.MustCompile("^/debug/pprof/trace$"), []string{"GET"}, authHandler(http.HandlerFunc(pprof.Trace), a))

	h.Handler(regexp.MustCompile("^/$"), []string{"GET"}, authHandler(http.HandlerFunc(renderHomepage), a))

	mux := forbidNonTLSTrafficHandler(h)
	mux = serverHeaderHandler(mux)
	mux = debugRequestBodyHandler(mux)
	return mux
}

func init() {
	DefaultServer = Get(Config{Auth: DefaultAuthorizer})
	disallowUnencryptedRequests = os.Getenv("ALLOW_UNENCRYPTED_PROXY_TRAFFIC") != "true"
}

func serverHeaderHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// hack, figure out how to put middleware on a subset of responses
		if strings.HasPrefix(r.URL.Path, "/debug/pprof") {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		} else if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		} else {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		w.Header().Set("Server", fmt.Sprintf("rickover/%s", config.Version))
		h.ServeHTTP(w, r)
	})
}

// forbidNonTLSTrafficHandler returns a 403 to traffic that is sent via a proxy
func forbidNonTLSTrafficHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if disallowUnencryptedRequests {
			if r.Header.Get("X-Forwarded-Proto") == "http" {
				// It should always be set, but if it's not, let the request
				// through.
				forbidden(w, insecure403(r))
				return
			}
		}
		// This header doesn't mean anything when served over HTTP, but
		// detecting HTTPS is a general way is hard, so let's just send it
		// every time.
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		h.ServeHTTP(w, r)
	})
}

func authHandler(h http.Handler, a Authorizer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userId, token, ok := r.BasicAuth()
		err := a.Authorize(userId, token)
		if err != nil {
			if !ok {
				authenticate(w, new401(r))
				return
			}
			metrics.Increment("auth.error")
			handleAuthorizeError(w, r, err)
			return
		}
		metrics.Increment("auth.success")
		h.ServeHTTP(w, r)
	})
}

// debugRequestBodyHandler prints all incoming and outgoing HTTP traffic if the
// DEBUG_HTTP_TRAFFIC environment variable is set to true. Note that the output
// will be jumbled if the server is handling multiple requests at the same
// time.
func debugRequestBodyHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("DEBUG_HTTP_TRAFFIC") == "true" {
			// You need to write the entire thing in one Write, otherwise the
			// output will be jumbled with other requests.
			b := new(bytes.Buffer)
			bits, err := httputil.DumpRequest(r, true)
			if err != nil {
				_, _ = b.WriteString(err.Error())
			} else {
				_, _ = b.Write(bits)
			}
			res := httptest.NewRecorder()
			h.ServeHTTP(res, r)

			_, _ = b.WriteString(fmt.Sprintf("HTTP/1.1 %d\r\n", res.Code))
			result := res.Result()
			_ = result.Header.Write(b)
			for k, v := range result.Header {
				w.Header()[k] = v
			}
			w.WriteHeader(res.Code)
			_, _ = b.WriteString("\r\n")
			writer := io.MultiWriter(w, b)
			_, _ = res.Body.WriteTo(writer)
			_, _ = b.WriteTo(os.Stderr)
		} else {
			h.ServeHTTP(w, r)
		}
	})
}

// CreateJobRequest is a struct of data sent in the body of a request to
// /v1/jobs
type CreateJobRequest struct {
	Name             string                     `json:"name"`
	Attempts         int16                      `json:"attempts"`
	Concurrency      int16                      `json:"concurrency"`
	DeliveryStrategy newmodels.DeliveryStrategy `json:"delivery_strategy"`
}

// GET /v1/jobs/:jobName
//
// Get a job type by name. Returns a models.Job or an error
func getJobType() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jobName := getJobTypeRoute.FindStringSubmatch(r.URL.Path)[1]
		job, err := jobs.Get(r.Context(), jobName)
		if err != nil {
			if err == sql.ErrNoRows {
				notFound(w, new404(r))
				return
			}
			rest.ServerError(w, r, err)
			return
		}
		respond(Logger, w, r, job)
	})
}

var Logger = handlers.Logger

// POST /v1/jobs
//
// createJob returns a http.HandlerFunc that responds to job creation requests
// using the given authorizer interface.
func createJob(useMetaShutdown bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			rest.BadRequest(w, r, createEmptyErr("name", r.URL.Path))
			return
		}
		defer r.Body.Close()
		var jr CreateJobRequest
		// XXX check for content-type
		err := json.NewDecoder(r.Body).Decode(&jr)
		if err != nil {
			rest.BadRequest(w, r, &rest.Error{
				ID:    "invalid_request",
				Title: "Invalid request: bad JSON. Double check the types of the fields you sent",
			})
			return
		}
		if jr.Name == "" {
			rest.BadRequest(w, r, createEmptyErr("name", r.URL.Path))
			return
		}
		if jr.Name == "meta.shutdown" {
			rest.BadRequest(w, r, &rest.Error{
				Title: "cannot create protected job name",
				ID:    "forbidden_parameter",
			})
			return
		}
		if jr.DeliveryStrategy == newmodels.DeliveryStrategy("") {
			rest.BadRequest(w, r, createEmptyErr("delivery_strategy", r.URL.Path))
			return
		}
		if jr.DeliveryStrategy != newmodels.DeliveryStrategyAtLeastOnce && jr.DeliveryStrategy != newmodels.DeliveryStrategyAtMostOnce {
			err := &rest.Error{
				Instance: r.URL.Path,
				ID:       "invalid_delivery_strategy",
				Title:    fmt.Sprintf("Invalid delivery strategy: %s", jr.DeliveryStrategy),
			}
			rest.BadRequest(w, r, err)
			return
		}

		if jr.DeliveryStrategy == newmodels.DeliveryStrategyAtMostOnce && jr.Attempts > 1 {
			err := &rest.Error{
				Instance: r.URL.Path,
				ID:       "invalid_attempts",
				Title:    "Cannot set retry attempts to a number greater than 1 if the delivery strategy is at_most_once",
				Detail:   "The at_most_once strategy implies only one attempt will be made.",
			}
			rest.BadRequest(w, r, err)
			return
		}

		if jr.Attempts == 0 {
			rest.BadRequest(w, r, createPositiveIntErr("Attempts", r.URL.Path))
			return
		}
		if jr.Concurrency == 0 {
			rest.BadRequest(w, r, createPositiveIntErr("Concurrency", r.URL.Path))
			return
		}

		jobData := newmodels.Job{
			Name:             jr.Name,
			DeliveryStrategy: jr.DeliveryStrategy,
			Concurrency:      jr.Concurrency,
			Attempts:         jr.Attempts,
		}
		start := time.Now()
		job, err := jobs.Create(newmodels.CreateJobParams{
			Name:             jobData.Name,
			DeliveryStrategy: jobData.DeliveryStrategy,
			Concurrency:      jobData.Concurrency,
			Attempts:         jobData.Attempts,
		})
		metrics.Time("type.create.latency", time.Since(start))
		if err != nil {
			switch terr := err.(type) {
			case *pq.Error:
				apierr := &rest.Error{
					Title:    terr.Message,
					ID:       "invalid_parameter",
					Instance: r.URL.Path,
				}
				rest.BadRequest(w, r, apierr)
				return
			default:
				rest.ServerError(w, r, err)
				return
			}
		}
		if useMetaShutdown {
			go func() {
				// The dequeuer won't know about the new job type, so enqueue
				// a shutdown job, which will instruct the dequeuer to shut down.
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				if err := createMetaShutdownJob(ctx); err != nil {
					return
				}
				id := types.GenerateUUID("job_")
				runAfter := time.Now().Add(-1 * 5 * time.Second)
				queued_jobs.Enqueue(newmodels.EnqueueJobParams{
					ID: id, Name: "meta.shutdown", RunAfter: runAfter,
					ExpiresAt: types.NullTime{
						Valid: true,
						Time:  runAfter.Add(time.Minute),
					},
					Data: []byte("{}"),
				})
			}()
		}
		created(Logger, w, r, job)
		metrics.Increment("type.create.success")
	})
}

// An EnqueueJobRequest is sent in the body of a request to PUT
// /v1/jobs/:job-name/:job-id.
type EnqueueJobRequest struct {
	// Job data to enqueue.
	Data json.RawMessage `json:"data"`
	// The earliest time we can run this job. If not specified, defaults to the
	// current time.
	RunAfter types.NullTime `json:"run_after"`
	// The latest time we can run this job. If not specified, defaults to null
	// (never expires).
	ExpiresAt types.NullTime `json:"expires_at"`
}

// GET/POST/PUT disambiguator for /v1/jobs/:name/:id
func handleJobRoute(useMetaShutdown bool) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			j := jobStatusUpdater{}
			j.ServeHTTP(w, r)
			return
		}
		if r.Method == "PUT" {
			j := jobEnqueuer{useMetaShutdown: useMetaShutdown}
			j.ServeHTTP(w, r)
			return
		}
		if r.Method == "GET" {
			j := jobStatusGetter{}
			j.ServeHTTP(w, r)
			return
		}
		rest.NotAllowed(w, r)
	})
}

type jobStatusGetter struct{}

// GET /v1/jobs(/:name)/:id
//
// Try to find the given job in the queued_jobs table, then in the
// archived_jobs table. Returns the job, or a 404 Not Found error.
func (j *jobStatusGetter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Job type, will be set if the longer URL form, empty string otherwise.
	var name string
	var idStr string

	// Try the longer route match first, fall back to just the ID
	jobIdMatch := jobIdRoute.FindStringSubmatch(r.URL.Path)
	if len(jobIdMatch) == 0 {
		jobIdMatch = getJobRoute.FindStringSubmatch(r.URL.Path)
		name = ""
		idStr = jobIdMatch[1]
	} else {
		name = jobIdMatch[1]
		idStr = jobIdMatch[2]
	}

	id, done := getId(w, r, idStr)
	if done {
		return
	}
	qj, err := queued_jobs.GetRetry(r.Context(), id, 3)
	if err == nil {
		if qj.Name != name && name != "" {
			// consider just serializing it if this is too annoying
			nfe := &rest.Error{
				Title:    "Job exists, but with a different name",
				ID:       "job_not_found",
				Instance: r.URL.Path,
			}
			notFound(w, nfe)
			return
		}
		respond(Logger, w, r, qj)
		metrics.Increment("job.get.queued.success")
		return
	}

	if err != queued_jobs.ErrNotFound {
		rest.ServerError(w, r, err)
		metrics.Increment("job.get.queued.error")
		return
	}

	aj, err := archived_jobs.GetRetry(id, 3)
	if err == archived_jobs.ErrNotFound {
		notFound(w, new404(r))
		metrics.Increment("job.get.not_found")
		return
	}
	if err != nil {
		rest.ServerError(w, r, err)
		return
	}
	respond(Logger, w, r, aj)
	metrics.Increment("job.get.archived.success")
}

func createMetaShutdownJob(ctx context.Context) error {
	// This job type might not exist, we need to create it first to avoid
	// a foreign key error. Just try to create the job every time - we
	// shouldn't be hitting this endpoint that often
	_, err := newmodels.DB.CreateJob(ctx, newmodels.CreateJobParams{
		Name:             "meta.shutdown",
		DeliveryStrategy: newmodels.DeliveryStrategyAtMostOnce,
		Attempts:         1,
		Concurrency:      1,
	})
	if err != nil {
		pqerr, ok := err.(*pq.Error)
		if !ok || pqerr.Code != "23505" {
			return err
		}
	}
	return nil
}

// jobEnqueuer satisfies the Handler interface.
type jobEnqueuer struct {
	useMetaShutdown bool
}

// PUT /v1/jobs/:name/:id
//
// Enqueue a new job.
func (j *jobEnqueuer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		rest.BadRequest(w, r, createEmptyErr("data", r.URL.Path))
		return
	}
	defer r.Body.Close()
	var ejr EnqueueJobRequest
	err := json.NewDecoder(r.Body).Decode(&ejr)
	if err != nil {
		rest.BadRequest(w, r, &rest.Error{
			ID:    "invalid_request",
			Title: "Invalid request: bad JSON. Double check the types of the fields you sent",
		})
		return
	}
	if ejr.Data == nil {
		rest.BadRequest(w, r, createEmptyErr("data", r.URL.Path))
		return
	}
	if !ejr.RunAfter.Valid {
		ejr.RunAfter = types.NullTime{
			Valid: true,
			Time:  time.Now().UTC(),
		}
	}
	matches := jobIdRoute.FindStringSubmatch(r.URL.Path)
	idStr := matches[2]
	var id types.PrefixUUID
	// Apache Bench can only hit one URL. This is a hack to allow random ID's
	// to be generated/inserted, even though the client is hitting the same
	// URL.
	//
	// Clients *must not* use random_id, they must generate their own UUID's.
	if idStr == "random_id" {
		id = types.GenerateUUID("job_")
		if err != nil {
			rest.ServerError(w, r, err)
			return
		}
	} else {
		var done bool
		id, done = getId(w, r, idStr)
		if done {
			return
		}
	}
	// TODO we've already consumed the whole thing, use limitedbytesreader
	// instead.
	if len(ejr.Data) > MAX_ENQUEUE_DATA_SIZE {
		tooLarge(w)
		return
	}
	name := jobIdRoute.FindStringSubmatch(r.URL.Path)[1]
	if name == "meta.shutdown" {
		createMetaShutdownJob(r.Context())
	}
	queuedJob, err := queued_jobs.Enqueue(newmodels.EnqueueJobParams{
		ID: id, Name: name, RunAfter: ejr.RunAfter.Time,
		ExpiresAt: ejr.ExpiresAt, Data: ejr.Data,
	})
	if err != nil {
		switch terr := err.(type) {
		case *queued_jobs.UnknownOrArchivedError:
			_, err = jobs.GetRetry(r.Context(), name, 3)
			if err != nil && err == sql.ErrNoRows {
				nfe := &rest.Error{
					Title:    fmt.Sprintf("Job type %s not found", name),
					ID:       "job_type_not_found",
					Instance: fmt.Sprintf("/v1/jobs/%s", name),
				}
				notFound(w, nfe)
				metrics.Increment(fmt.Sprintf("enqueue.%s.not_found", name))
				return
			} else {
				alreadyArchived := &rest.Error{
					Title:    "Job has already been archived",
					ID:       "job_already_archived",
					Instance: fmt.Sprintf("/v1/jobs/%s/%s", name, id.String()),
				}
				rest.BadRequest(w, r, alreadyArchived)
				metrics.Increment("enqueue.error.already_archived")
				return
			}
		case *pq.Error:
			if terr.Code == "23505" { // unique violation
				queuedJob, err = queued_jobs.Get(r.Context(), id)
				if err != nil {
					rest.ServerError(w, r, err)
					return
				}
				break
			}
			apierr := &rest.Error{
				Title:    terr.Message,
				ID:       "invalid_parameter",
				Instance: r.URL.Path,
			}
			rest.BadRequest(w, r, apierr)
			metrics.Increment(fmt.Sprintf("enqueue.%s.failure", name))
			return
		default:
			rest.ServerError(w, r, err)
			metrics.Increment(fmt.Sprintf("enqueue.%s.error", name))
			return
		}
	}
	accepted(Logger, w, r, queuedJob)
	metrics.Increment("enqueue.success")
	metrics.Increment("enqueue." + name + ".success")
}

// GET /v1/archived-jobs
func listArchivedJobs() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		typ := query.Get("name")
		status := query.Get("status")
		// TODO validate status
		limitQ := query.Get("limit")
		var limit int
		if limitQ == "" {
			limit = 100
		} else {
			var err error
			limit, err = strconv.Atoi(limitQ)
			if err != nil {
				rest.BadRequest(w, r, &rest.Error{Title: err.Error(), ID: "invalid_parameter"})
				return
			}
			if limit <= 0 {
				rest.BadRequest(w, r, &rest.Error{
					Title: "limit cannot be negative", ID: "invalid_parameter",
				})
				return
			}
			if limit > 100 {
				rest.BadRequest(w, r, &rest.Error{
					Title: "limit cannot be greater than 100", ID: "invalid_parameter",
				})
				return
			}
		}
		var ajs []newmodels.ArchivedJob
		var err error
		switch {
		case typ == "" && status == "":
			ajs, err = newmodels.DB.ListArchivedJobs(r.Context(), int32(limit))
		case status == "" && typ != "":
			ajs, err = newmodels.DB.ListArchivedJobsByName(r.Context(), newmodels.ListArchivedJobsByNameParams{
				Limit: int32(limit),
				Name:  typ,
			})
		case status != "" && typ == "":
			ajs, err = newmodels.DB.ListArchivedJobsByStatus(r.Context(), newmodels.ListArchivedJobsByStatusParams{
				Limit:  int32(limit),
				Status: newmodels.ArchivedJobStatus(status),
			})
		case status != "" && typ != "":
			ajs, err = newmodels.DB.ListArchivedJobsByNameStatus(r.Context(), newmodels.ListArchivedJobsByNameStatusParams{
				Limit:  int32(limit),
				Name:   typ,
				Status: newmodels.ArchivedJobStatus(status),
			})
		default:
			panic("should be unreachable")
		}
		if err != nil {
			rest.ServerError(w, r, err)
			return
		}
		// TODO: add Next, Previous pagination by encrypted auto id
		respond(Logger, w, r, ajs)
	})
}
