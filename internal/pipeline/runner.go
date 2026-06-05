package pipeline

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/elythi0n/virta/internal/clock"
	"github.com/elythi0n/virta/internal/platform"
)

// Default buffer sizes.
const (
	defaultIngestBuffer = 4096
	defaultSinkBuffer   = 5000
)

// Options configure a Runner.
type Options struct {
	Clock        clock.Clock // time source for ReceivedAt stamping (required)
	Stages       []Stage     // ordered, pure annotation stages
	Sinks        []Sink      // concurrent consumers
	IngestBuffer int         // ingest queue depth (default 4096)
	SinkBuffer   int         // per-sink ring buffer depth (default 5000)
	Logger       *slog.Logger
}

// Runner is the concrete Pipeline. One dispatcher goroutine stamps ReceivedAt,
// runs the ordered stages on each message (panic-isolated), then hands every event to each
// sink's own ring buffer. Each sink drains its buffer in its own goroutine, so a slow sink
// only loses its own oldest events (counted) and never stalls the dispatcher or other sinks.
//
// Ingest is lossless: Submit blocks only if the ingest queue is saturated, which pure/fast
// stages make rare. Loss happens only at the per-sink boundary, by design.
type Runner struct {
	clk    clock.Clock
	stages []Stage
	log    *slog.Logger

	in   chan platform.Event
	quit chan struct{}
	ctx  context.Context
	stop context.CancelFunc

	sinks []*sinkWorker

	wg        sync.WaitGroup // dispatcher + sink workers
	attachWG  sync.WaitGroup // fan-in forwarders
	closeOnce sync.Once

	dispatched  atomic.Int64
	stagePanics atomic.Int64
	stageErrors atomic.Int64
	submitDrops atomic.Int64 // events dropped because Submit raced Close
}

// NewRunner builds a runner. Call Start before submitting.
func NewRunner(opts Options) *Runner {
	if opts.IngestBuffer <= 0 {
		opts.IngestBuffer = defaultIngestBuffer
	}
	if opts.SinkBuffer <= 0 {
		opts.SinkBuffer = defaultSinkBuffer
	}
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	ctx, stop := context.WithCancel(context.Background())
	r := &Runner{
		clk:    opts.Clock,
		stages: opts.Stages,
		log:    log,
		in:     make(chan platform.Event, opts.IngestBuffer),
		quit:   make(chan struct{}),
		ctx:    ctx,
		stop:   stop,
	}
	for _, s := range opts.Sinks {
		r.sinks = append(r.sinks, newSinkWorker(s, opts.SinkBuffer))
	}
	return r
}

// Start launches the dispatcher and sink workers.
func (r *Runner) Start() {
	for _, sw := range r.sinks {
		r.wg.Add(1)
		go func(sw *sinkWorker) {
			defer r.wg.Done()
			sw.run(r.ctx, r.quit, r.log)
		}(sw)
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.dispatch()
	}()
}

// Attach forwards an adapter's event channel into the pipeline (fan-in). It returns once a
// forwarder goroutine is running; the goroutine exits when ch closes or the runner stops.
func (r *Runner) Attach(ch <-chan platform.Event) {
	r.attachWG.Add(1)
	go func() {
		defer r.attachWG.Done()
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return
				}
				r.Submit(ev)
			case <-r.quit:
				return
			}
		}
	}()
}

// Submit enqueues an event from any producer. Blocks only if the ingest queue is full;
// drops (counted) only if the runner is shutting down.
func (r *Runner) Submit(ev platform.Event) {
	select {
	case r.in <- ev:
	case <-r.quit:
		r.submitDrops.Add(1)
	}
}

func (r *Runner) dispatch() {
	for {
		select {
		case ev := <-r.in:
			r.process(ev)
		case <-r.quit:
			// drain whatever is already queued, then exit
			for {
				select {
				case ev := <-r.in:
					r.process(ev)
				default:
					return
				}
			}
		}
	}
}

func (r *Runner) process(ev platform.Event) {
	if me, ok := ev.(platform.MessageEvent); ok {
		msg := me.Message
		if msg.ReceivedAt.IsZero() {
			msg.ReceivedAt = r.clk.Now()
		}
		if r.runStages(&msg) {
			return // dropped to diagnostics; do not fan out
		}
		ev = platform.MessageEvent{Message: msg}
	}
	r.dispatched.Add(1)
	for _, sw := range r.sinks {
		sw.push(ev)
	}
}

// runStages applies each stage in order; returns true if the message was dropped (stage
// panic or error). A panic is recovered so a buggy stage drops only its own message, never the whole dispatcher.
func (r *Runner) runStages(msg *platform.UnifiedMessage) (dropped bool) {
	for _, st := range r.stages {
		if r.runStage(st, msg) {
			return true
		}
	}
	return false
}

func (r *Runner) runStage(st Stage, msg *platform.UnifiedMessage) (dropped bool) {
	defer func() {
		if rec := recover(); rec != nil {
			r.stagePanics.Add(1)
			r.log.Error("pipeline stage panic", "stage", st.Name(), "panic", rec)
			dropped = true
		}
	}()
	if err := st.Annotate(r.ctx, msg); err != nil {
		r.stageErrors.Add(1)
		r.log.Warn("pipeline stage error", "stage", st.Name(), "err", err)
		return true
	}
	return false
}

// Close stops the pipeline: halts fan-in, drains the ingest queue, stops sink workers, and
// closes every sink. Idempotent.
func (r *Runner) Close() error {
	r.closeOnce.Do(func() {
		close(r.quit) // stop forwarders, Submit, and signal dispatcher/workers to drain
		r.attachWG.Wait()
		r.wg.Wait() // dispatcher + sink workers finished draining
		r.stop()    // cancel ctx for any in-flight Consume
		for _, sw := range r.sinks {
			if err := sw.sink.Close(); err != nil {
				r.log.Warn("sink close error", "sink", sw.sink.Name(), "err", err)
			}
		}
	})
	return nil
}

// Stats is a snapshot of runner counters.
type Stats struct {
	Dispatched  int64
	StagePanics int64
	StageErrors int64
	SubmitDrops int64
	SinkDrops   map[string]int64
	SinkErrors  map[string]int64
}

// Stats returns current counters.
func (r *Runner) Stats() Stats {
	s := Stats{
		Dispatched:  r.dispatched.Load(),
		StagePanics: r.stagePanics.Load(),
		StageErrors: r.stageErrors.Load(),
		SubmitDrops: r.submitDrops.Load(),
		SinkDrops:   map[string]int64{},
		SinkErrors:  map[string]int64{},
	}
	for _, sw := range r.sinks {
		s.SinkDrops[sw.sink.Name()] = sw.drops.Load()
		s.SinkErrors[sw.sink.Name()] = sw.errors.Load()
	}
	return s
}

var _ Pipeline = (*Runner)(nil)

// ---- per-sink worker with a drop-oldest ring buffer ----

type sinkWorker struct {
	sink   Sink
	ch     chan platform.Event
	drops  atomic.Int64
	errors atomic.Int64
}

func newSinkWorker(s Sink, buf int) *sinkWorker {
	return &sinkWorker{sink: s, ch: make(chan platform.Event, buf)}
}

// push enqueues without blocking the dispatcher. If the buffer is full it drops the oldest
// queued event (counting it) and retries — so the newest events always make it in.
func (w *sinkWorker) push(ev platform.Event) {
	for {
		select {
		case w.ch <- ev:
			return
		default:
			select {
			case <-w.ch:
				w.drops.Add(1)
			default:
				// raced empty; loop and try to send again
			}
		}
	}
}

func (w *sinkWorker) run(ctx context.Context, quit <-chan struct{}, log *slog.Logger) {
	consume := func(ev platform.Event) {
		if err := w.sink.Consume(ctx, ev); err != nil {
			w.errors.Add(1)
			log.Warn("sink consume error", "sink", w.sink.Name(), "err", err)
		}
	}
	for {
		select {
		case ev := <-w.ch:
			consume(ev)
		case <-quit:
			// drain remaining buffered events, then exit
			for {
				select {
				case ev := <-w.ch:
					consume(ev)
				default:
					return
				}
			}
		}
	}
}
