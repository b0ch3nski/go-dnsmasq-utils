package dnsmasq

import (
	"bufio"
	"context"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	logBufferSize = 1000
	queryTimeout  = 5 * time.Second
)

var (
	logStart   = `dnsmasq\[\d+\]: \d+ (.+)\/(.+) `
	rgxQuery   = regexp.MustCompile(logStart + `query\[A+\] (.+) from`)
	rgxForward = regexp.MustCompile(logStart + `forwarded.+to (.+)`)
	rgxReply   = regexp.MustCompile(logStart + `(?:reply|cached).+is (.+)`)
)

// LogFn defines a function executed on each log line read from dnsmasq with ability to distinguish if the line was
// related to DNS query.
type LogFn func(line string, dns bool)

// Query represents DNS query served by dnsmasq.
type Query struct {
	Domain   string
	MadeBy   string
	Queried  []string
	Result   []string
	Started  time.Time
	Finished time.Time
}

// WatchLogs reads dnsmasq logs from a named pipe (FIFO) and sends all completed DNS queries over output channel.
// Optionally pass func(string, bool) to access each incoming log line.
// Function is blocking until context is done.
func WatchLogs(ctx context.Context, filePath string, output chan<- *Query, fn LogFn) error {
	defer close(output)

	if _, errStat := os.Stat(filePath); os.IsNotExist(errStat) {
		if errFifo := syscall.Mkfifo(filePath, 0644); errFifo != nil {
			return errFifo
		}
	}
	// open pipe in RW mode to avoid EOF when the only one writer disconnects
	pipeFile, errOpen := os.OpenFile(filePath, os.O_RDWR, os.ModeNamedPipe)
	if errOpen != nil {
		return errOpen
	}
	go func() {
		<-ctx.Done()
		pipeFile.Close()
	}()

	store := &queryStore{
		ongoing:   make(map[string]*atomicQuery, 100),
		completed: output,
		timeout:   queryTimeout,
	}

	for logLine := range readPipeToChan(ctx, pipeFile) {
		dnsMatch := processLine(ctx, store, logLine)
		if fn != nil {
			fn(logLine.line, dnsMatch)
		}
	}

	store.routines.Wait()
	return nil
}

func readPipeToChan(ctx context.Context, reader io.Reader) <-chan *logLine {
	ch := make(chan *logLine, logBufferSize)

	go func() {
		defer close(ch)

		bufRd := bufio.NewReader(reader)
		for {
			if ctx.Err() != nil {
				return
			}

			line, _ := bufRd.ReadString('\n')
			if line = strings.TrimSpace(line); line == "" {
				continue
			}
			ll := &logLine{line: line, date: time.Now()}

			select {
			case ch <- ll:
			default: // when buffer is full, read (drop) one item
				<-ch
				ch <- ll
			}
		}
	}()

	return ch
}

func processLine(ctx context.Context, store *queryStore, ll *logLine) bool {
	if matchQuery := rgxQuery.FindStringSubmatch(ll.line); matchQuery != nil {
		store.newQuery(ctx, matchQuery[2], matchQuery[1], matchQuery[3], ll.date)
		return true
	}

	// forward is optional - won't appear when query is cached
	// but can appear multiple times when subsequent servers failed to respond
	if matchForward := rgxForward.FindStringSubmatch(ll.line); matchForward != nil {
		store.appendQueried(matchForward[2], matchForward[3])
		return true
	}

	if matchReply := rgxReply.FindStringSubmatch(ll.line); matchReply != nil {
		store.appendResult(matchReply[2], matchReply[3], ll.date)
		return true
	}

	return false
}

type logLine struct {
	line string
	date time.Time
}

type atomicQuery struct {
	Query
	gate uint32
}

type queryStore struct {
	ongoing   map[string]*atomicQuery
	completed chan<- *Query
	timeout   time.Duration
	routines  sync.WaitGroup
	sync.RWMutex
}

func (qs *queryStore) newQuery(ctx context.Context, id, madeBy, domain string, start time.Time) {
	q := &atomicQuery{Query: Query{
		MadeBy:  madeBy,
		Domain:  domain,
		Queried: make([]string, 0, 1),
		Result:  make([]string, 0, 1),
		Started: start,
	}}
	qs.Lock()
	qs.ongoing[id] = q
	qs.Unlock()

	qs.routines.Add(1)
	go func() {
		defer qs.routines.Done()

		select {
		case <-ctx.Done():
		case <-time.After(qs.timeout):
		}

		atomic.SwapUint32(&q.gate, 1) // close gate
		if len(q.Result) > 0 {
			qs.completed <- &q.Query
		}

		qs.Lock()
		delete(qs.ongoing, id)
		qs.Unlock()
	}()
}

func (qs *queryStore) get(id string) (*atomicQuery, bool) {
	qs.RLock()
	q, ok := qs.ongoing[id]
	qs.RUnlock()
	return q, ok
}

func (qs *queryStore) appendQueried(id, queried string) {
	q, ok := qs.get(id)
	if !ok || !atomic.CompareAndSwapUint32(&q.gate, 0, 1) {
		return
	}
	q.Queried = append(q.Queried, queried)
	atomic.StoreUint32(&q.gate, 0)
}

func (qs *queryStore) appendResult(id, result string, end time.Time) {
	q, ok := qs.get(id)
	if !ok || !atomic.CompareAndSwapUint32(&q.gate, 0, 1) {
		return
	}
	q.Result = append(q.Result, result)
	q.Finished = end
	atomic.StoreUint32(&q.gate, 0)
}
