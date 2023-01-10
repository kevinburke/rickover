semaphore
=========

This implements a semaphore in Go. Use this to manage the maximum amount of
concurrency you want.

[![GoDoc](https://godoc.org/github.com/kevinburke/semaphore?status.svg)](https://godoc.org/github.com/kevinburke/semaphore)

### Usage

To create a new Semaphore with 5 workers:

```go
import "github.com/kevinburke/semaphore"
...
sem := semaphore.New(5) // new semaphore with 5 permits
```

Acquire one of the workers:

```go
sem.Acquire() // one
// Acquire with a timeout, or cancelable context:
sem.AcquireContext(context.TODO())
```

Once you're done with the semaphore:

```go
sem.Release() // Release one worker
sem.Drain() // Release them all
```

### Complete documentation

See here: https://godoc.org/github.com/kevinburke/semaphore
