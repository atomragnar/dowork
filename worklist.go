package dowork

import (
	"sync"
)

type Job interface {
	Do() error
}

type WorkIsDoneError struct {
	Message string
}

func (w *WorkIsDoneError) Error() string {
	return w.Message
}

// finished
type finshedJob struct {
	done func() error
}

func (f *finshedJob) Do() error {
	return f.done()
}

func newFinishedJob() *finshedJob {
	return &finshedJob{
		done: func() error {
			return &WorkIsDoneError{"Job is done"}
		},
	}
}

type WorkList struct {
	jobs      chan Job
	closed    bool
	closelock sync.Mutex
}

// New initializes a new WorkList with a specified capacity for the jobs channel
func NewWorkList(capacity int) *WorkList {
	return &WorkList{
		jobs: make(chan Job, capacity),
	}
}

func (w *WorkList) Add(job Job) {
	w.jobs <- job
}

// Next dequeues a job from the workList, returning false if no jobs are left
func (w *WorkList) Next() (Job, bool) {
	w.closelock.Lock()
	defer w.closelock.Unlock()

	job, ok := <-w.jobs
	if !ok {
		return nil, false // Channel is closed, no more jobs
	}

	// If a job signals that work is done, initiate cleanup
	if _, ok := job.(*finshedJob); ok {
		if !w.closed {
			close(w.jobs)
			w.closed = true
		}
		return nil, false // Signal to worker that no more jobs will come
	}

	return job, true
}

// We add a `NoMoreJobs` message to the workList for each
// worker that we are using. Once the worker receives this
// message, it will terminate. After each worker terminates,
// the program can continue.
func (w *WorkList) Finalize(numWorkers int) {
	for i := 0; i < numWorkers; i++ {
		w.Add(newFinishedJob())
	}
}

// Close safely closes the jobs channel, ensuring no more jobs are added
func (w *WorkList) Close() {
	w.closelock.Lock()
	defer w.closelock.Unlock()
	if !w.closed {
		close(w.jobs)
		w.closed = true
	}
}

func (w *WorkList) Jobs() <-chan Job {
	return w.jobs
}

func (w *WorkList) Len() int {
	return len(w.jobs)
}

func (w *WorkList) IsEmpty() bool {
	return w.Len() == 0
}

func (w *WorkList) IsClosed() bool {
	select {
	case _, ok := <-w.jobs:
		return !ok
	default:
		return false
	}
}

func (w *WorkList) Wait() {
	for !w.IsClosed() {
	}
}
