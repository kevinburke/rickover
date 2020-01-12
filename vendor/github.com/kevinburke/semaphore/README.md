
semaphore
=========

Semaphore implementation in golang

[![GoDoc](https://godoc.org/github.com/kevinburke/semaphore?status.svg)](https://godoc.org/github.com/kevinburke/semaphore)

### Usage

Initiate

```go
import "github.com/kevinburke/semaphore"
...
sem := semaphore.New(5) // new semaphore with 5 permits
```

Acquire

```go
sem.Acquire() // one
sem.AcquireContext(context.Background())
```

Release

```go
sem.Release() // one
sem.Drain() // all
```

### documentation

See here: https://godoc.org/github.com/kevinburke/semaphore
