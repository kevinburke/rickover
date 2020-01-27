package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	uuid "github.com/kevinburke/go.uuid"
)

type ctxVar int

var requestID ctxVar = 0
var startTime ctxVar = 1

// SetRequestID sets the given UUID on the request context and returns the
// modified HTTP request.
func SetRequestID(r *http.Request, u uuid.UUID) *http.Request {
	r2 := new(http.Request)
	*r2 = *r
	r2.Header.Set("X-Request-Id", u.String())
	return r2.WithContext(context.WithValue(r2.Context(), requestID, u))
}

// GetRequestID returns a UUID (if it exists in the context) or false if none
// could be found.
func GetRequestID(ctx context.Context) (uuid.UUID, bool) {
	val := ctx.Value(requestID)
	if val != nil {
		v, ok := val.(uuid.UUID)
		return v, ok
	}
	return uuid.UUID{}, false
}

// GetDuration returns the amount of time since the Duration handler ran, or
// 0 if no Duration was set for this context.
func GetDuration(ctx context.Context) time.Duration {
	t := getStart(ctx)
	if t.IsZero() {
		return time.Duration(0)
	}
	return time.Since(t)
}

// GetStartTime returns the time the Duration handler ran.
func GetStartTime(ctx context.Context) time.Time {
	val := ctx.Value(startTime)
	if val != nil {
		t := val.(time.Time)
		return t
	}
	return time.Time{}
}

// getStart returns the time the Duration handler ran.
func getStart(ctx context.Context) time.Time {
	val := ctx.Value(startTime)
	if val != nil {
		t := val.(time.Time)
		return t
	}
	return time.Time{}
}

type startWriter struct {
	w           http.ResponseWriter
	start       time.Time
	wroteHeader bool
}

func (s *startWriter) duration() string {
	d := time.Since(s.start) / (100 * time.Microsecond) * 100 * time.Microsecond
	return strings.Replace(d.String(), "µ", "u", 1)
}

func (s *startWriter) WriteHeader(code int) {
	//lint:ignore S1002 prefer it this way
	if s.wroteHeader == false {
		s.w.Header().Set("X-Request-Duration", s.duration())
		s.wroteHeader = true
	}
	s.w.WriteHeader(code)
}

func (s *startWriter) Write(b []byte) (int, error) {
	// Some chunked encoding transfers won't ever call WriteHeader(), so set
	// the header here.
	//lint:ignore S1002 prefer it this way
	if s.wroteHeader == false {
		s.w.Header().Set("X-Request-Duration", s.duration())
		s.wroteHeader = true
	}
	return s.w.Write(b)
}

func (s *startWriter) Header() http.Header {
	return s.w.Header()
}

// Push implements the http.Pusher interface.
func (s *startWriter) Push(target string, opts *http.PushOptions) error {
	return push(s.w, target, opts)
}

// Duration sets the start time in the context and sets a X-Request-Duration
// header on the response, from the time this handler started executing to the
// time of the first WriteHeader() or Write() call.
func Duration(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &startWriter{
			w:           w,
			start:       time.Now().UTC(),
			wroteHeader: false,
		}
		r2 := new(http.Request)
		*r2 = *r
		r2 = r2.WithContext(context.WithValue(r2.Context(), startTime, sw.start))
		h.ServeHTTP(sw, r2)
		//lint:ignore S1002 prefer it this way
		if sw.wroteHeader == false {
			// never called Write() or WriteHeader()
			sw.w.Header().Set("X-Request-Duration", sw.duration())
			sw.wroteHeader = true
		}
	})
}

// WithTimeout sets the given timeout in the Context of every incoming request
// before passing it to the next handler.
func WithTimeout(h http.Handler, timeout time.Duration) http.Handler {
	if timeout < 0 {
		panic("invalid timeout (negative number)")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		r2 := new(http.Request)
		*r2 = *r
		r2 = r2.WithContext(ctx)
		h.ServeHTTP(w, r2)
	})
}
