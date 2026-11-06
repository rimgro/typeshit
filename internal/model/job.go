package model

import "time"

// JobStatus is the lifecycle state of a generation job.
type JobStatus string

const (
	JobQueued  JobStatus = "queued"
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
	JobError   JobStatus = "error"
)

// Job is an asynchronous generation task.
type Job struct {
	ID      string          `json:"id"`
	Status  JobStatus       `json:"status"`
	Created time.Time       `json:"created"`
	Updated time.Time       `json:"updated"`
	Request GenerateRequest `json:"request"`
	Pages   []string        `json:"pages"` // file names inside the result dir
	Width   int             `json:"width"`
	Height  int             `json:"height"`
	Error   string          `json:"error,omitempty"`
	Millis  int64           `json:"millis"`
}
