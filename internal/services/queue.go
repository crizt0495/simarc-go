package services

import (
	"log"
	"sync"
	"time"

	"arsippro/internal/database"
)

type Job struct {
	Type    string
	Payload map[string]interface{}
}

type JobQueue struct {
	mu      sync.Mutex
	queue   []Job
	workers int
	wg      sync.WaitGroup
	stopCh  chan struct{}
	handler func(Job)
}

var GlobalQueue *JobQueue

// InitQueue starts the global job queue. It is safe to call multiple times
// (e.g. after reconnecting a database) — workers are only started once.
func InitQueue(workerCount int, handler func(Job)) {
	if GlobalQueue != nil {
		return
	}
	GlobalQueue = &JobQueue{
		queue:   make([]Job, 0),
		workers: workerCount,
		stopCh:  make(chan struct{}),
		handler: handler,
	}
	GlobalQueue.Start()
}

func (q *JobQueue) Start() {
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}
	log.Printf("Job queue started with %d workers", q.workers)
}

func (q *JobQueue) Stop() {
	close(q.stopCh)
	q.wg.Wait()
	log.Println("Job queue stopped")
}

func (q *JobQueue) Enqueue(job Job) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = append(q.queue, job)
}

func (q *JobQueue) dequeue() *Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue) == 0 {
		return nil
	}
	job := q.queue[0]
	q.queue = q.queue[1:]
	return &job
}

func (q *JobQueue) worker(id int) {
	defer q.wg.Done()
	for {
		select {
		case <-q.stopCh:
			return
		default:
			job := q.dequeue()
			if job == nil {
				continue
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[Worker %d] Recovered from panic in job %s: %v", id, job.Type, r)
					}
				}()
				log.Printf("[Worker %d] Processing job: %s", id, job.Type)
				q.handler(*job)
			}()
		}
	}
}

// ProcessJob is the default job handler
func ProcessJob(job Job) {
	switch job.Type {
	case "ocr_process":
		// OCR processing is handled inline for now
	case "backup_database":
		svc := &BackupService{}
		if _, err := svc.CreateDatabaseBackup(); err != nil {
			log.Printf("Backup job failed: %v", err)
		}
	case "import_excel":
		// Import processing
	case "export_data":
		// Export processing
	default:
		log.Printf("Unknown job type: %s", job.Type)
	}
}

var autoDisposalOnce sync.Once

// StartAutoDisposal starts the periodic auto-disposal background job
// (recalculate retention dates + check pemusnahan). Safe to call multiple
// times — the loop is only started once per process.
func StartAutoDisposal() {
	autoDisposalOnce.Do(func() {
		go func() {
			autoSvc := &AutoDisposalService{}
			// Run immediately at startup (skip when the DB is unreachable —
			// the app may be in recovery mode)
			if database.Connected() {
				autoSvc.RecalculateRetentionDates()
				autoSvc.CheckAndCreatePemusnahan()
			}
			// Then run periodically every 24 hours
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				if !database.Connected() {
					continue
				}
				autoSvc.RecalculateRetentionDates()
				autoSvc.CheckAndCreatePemusnahan()
			}
		}()
		log.Println("Auto-disposal background job started")
	})
}

// EnqueueJob adds a job to the global queue
func EnqueueJob(jobType string, payload map[string]interface{}) {
	if GlobalQueue == nil {
		log.Println("Queue not initialized, running job synchronously")
		ProcessJob(Job{Type: jobType, Payload: payload})
		return
	}
	GlobalQueue.Enqueue(Job{Type: jobType, Payload: payload})
}
