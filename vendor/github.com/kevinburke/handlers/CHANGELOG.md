### 0.38

If WriteHeader is never called, Log will log status=200 to the log, instead of
status=0.

### 0.37

Support OPTIONS queries with a nil route.

### 0.35

If two paths are declared that both match a given path, we'll try both of them
before giving up and returning a HTTP 405. This is slightly slower (we have to
try every route before giving up), but does the right thing.

### 0.34

Pass nil to specify you want to handle all HTTP methods in the router.

### 0.33

Make copies of all HTTP requests before passing them further down the chain. Per
the documentation, this is the correct way to handle this situation.

### 0.32

Fix an error in the duration information reported by X-Request-Duration.
