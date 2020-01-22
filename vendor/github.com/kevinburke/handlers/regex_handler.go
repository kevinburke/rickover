package handlers

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/kevinburke/rest"
)

type route struct {
	pattern *regexp.Regexp
	methods []string
	handler http.Handler
}

// A RegexpHandler is a simple http.Handler that can match regular expressions
// for routes.
type Regexp struct {
	routes []*route
}

// Handle calls the provided handler for requests whose URL matches the given
// pattern and HTTP method. The first matching route will get called. If methods
// is nil, all HTTP methods will be allowed. If GET is in the list of methods,
// HEAD requests will also be allowed.
func (h *Regexp) Handle(pattern *regexp.Regexp, methods []string, handler http.Handler) {
	h.routes = append(h.routes, &route{
		pattern: pattern,
		methods: methods,
		handler: handler,
	})
}

// HandleFunc calls the provided HandlerFunc for requests whose URL matches the
// given pattern and HTTP method. The first matching route will get called.
// If methods is nil, all HTTP methods are allowed. If GET is in the list of
// methods, HEAD requests will also be allowed.
func (h *Regexp) HandleFunc(pattern *regexp.Regexp, methods []string, handler func(http.ResponseWriter, *http.Request)) {
	h.routes = append(h.routes, &route{
		pattern: pattern,
		methods: methods,
		handler: http.HandlerFunc(handler),
	})
}

var allMethods = []string{
	"GET",
	"POST",
	"PUT",
	"PATCH",
	"DELETE",
	"CONNECT",
	"TRACE",
}

// ServeHTTP checks all registered routes in turn for a match, and calls
// handler.ServeHTTP on the first matching handler. If no routes match,
// StatusMethodNotAllowed will be rendered.
func (h *Regexp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upperMethod := strings.ToUpper(r.Method)
	allowed := make([]string, 0)
	oneMatch := false
	for _, route := range h.routes {
		if route.pattern.MatchString(r.URL.Path) {
			oneMatch = true
			if route.methods == nil && upperMethod != "OPTIONS" {
				route.handler.ServeHTTP(w, r)
				return
			}
			for _, method := range route.methods {
				upper := strings.ToUpper(method)
				if upper == upperMethod || upperMethod == "HEAD" && upper == "GET" {
					route.handler.ServeHTTP(w, r)
					return
				}
			}
			if upperMethod == "OPTIONS" {
				allowed = append(allowed, route.methods...)
			}
		}
	}
	if upperMethod == "OPTIONS" {
		var methods string
		if len(allowed) > 0 {
			methods = strings.Join(append(allowed, "OPTIONS"), ", ")
		} else {
			methods = strings.Join(append(allMethods, "OPTIONS"), ", ")
		}
		w.Header().Set("Allow", methods)
		return
	}
	if oneMatch {
		rest.NotAllowed(w, r)
	} else {
		rest.NotFound(w, r)
	}
}
