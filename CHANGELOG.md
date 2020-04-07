## Version 2.0

- Requests without Basic Auth credentials will only receive a 401 if the
Authorizer returns an error.

- Get with a nil Authorizer will panic.

- Use `pg_advisory_xact_lock` to lock queued jobs before marking them as in
progress, instead of selecting a row for update.

- Removed go-dberror due to lack of a LICENSE. As a result, error messages and
  types may change.

- Removed Sleep() interface from the Worker.

- JobProcessor now has a Handler interface that is more flexible for how work is
  to be performed. NewJobProcessor accepts a Handler; the old downstreamUrl and
  Password can be created by calling

    ```
    NewJobProcessor(NewDownstreamHandler(url, password))
    ```

- Many function calls, including database queries and the downstream client,
now accept a `context.Context`, including the dequeuer, which no longer has a
`QuitChan`. Use a `context.CancelFunc` to cancel the dequeuer.

- The `services.JobProcessor` does not automatically use a `downstream.Client`,
it has a `Handle(context.Context, *QueuedJob)` interface that can be used to
swap out the implementation of the thing that actually processes the downstream
job.

- A new `WorkServer` struct in the `dequeuer` package is designed to simplify
configuration of the deqeueuer. Call `dequeuer.New()` with a `dequeuer.Config`
to retrieve a `WorkServer`, then call `Run()` on the `WorkServer` to start the
dequeuer, the stuck job watcher, and all metrics observers.

    See the Example in the top level package for more usage guidance.

- If we attempted to record a failed job but the attempt numbers don't match up
with what's in the database, previously we would return a 500. Now we catch the
error and return an appropriate 400.

- Use log15 instead of the standard library logger. Some functions now accept
a `log.Logger` as an argument. Use `log15.New()` to create a Logger to pass to
these functions.

- A new Metrics interface exists - override metrics.Client with your own
implementation to control where metrics are sent. See the README for more
information.

## Version 0.37

Fixes a crashing error in dequeuer.CreatePools (and adds tests so it can't
happen again)

## Version 0.36

The `job_` prefix is now retrieved as part of the database query instead of
being attached in the model. You'll need to update `go-types` to version 0.13
at the latest.

## Version 0.35

- Fixes an error in a test committed shortly before version 0.34.

## Version 0.34

- Support marking failed jobs as un-retryable; pass `{"status": "failed",
"retryable": false}` in your status callback endpoint to immediately archive
the job.

- The 0.33 git tag doesn't compile due to the error fixed here:
https://github.com/Shyp/bump_version/commit/2dc60a73949ae5e42468d475a90e76619dbc67a6.
Adds regression tests to ensure this doesn't happen again.

## Version 0.33

- All uses of `Id` have been renamed to `ID`, per the Go Code Review Comments
guidelines. I don't like breaking this, but I'd rather keep the naming
idiomatic, Go will detect incorrect references at compile time, and I haven't
received any evidence that anyone else is using the project, so I am not too
worried about breaking compatibility in the wild.

- When replaying a job, use the `expires_at` value from the old job to
  determine whether to re-run it.

- Enabled several skipped tests and improved their speed/reduced their size.

## Version 0.32

- The `archived_jobs` table now has an `expires_at` column storing when the job
expired, or should have expired. This is useful for replaying jobs - you can
batch replay jobs and the server will correctly mark them as expired.
