package dnsmasq

import (
	"context"
	"fmt"
	"os"
	"path"
	"syscall"
	"testing"
	"time"
)

const exampleLog1 = `
Feb  4 18:18:18 dnsmasq[1]: 1 127.0.0.1/123 query[A] google.com from 127.0.0.1
Feb  4 18:18:18 dnsmasq[1]: 1 127.0.0.1/123 forwarded google.com to 1.1.1.1
Feb  4 18:18:18 dnsmasq[1]: 1 127.0.0.1/123 reply google.com is 8.8.8.8
`

const exampleLog2 = `
Feb  4 19:19:19 dnsmasq[1]: 2 192.168.1.1/321 query[A] cf-dns from 192.168.1.1
Feb  4 19:19:19 dnsmasq[1]: 2 192.168.1.1/321 cached cf-dns is 1.1.1.1
Feb  4 19:19:19 dnsmasq[1]: 2 192.168.1.1/321 cached cf-dns is 1.0.0.1
`

func TestWatchLogs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	pipePath := path.Join(os.TempDir(), fmt.Sprintf("log-pipe-%d", time.Now().Unix()))

	if errFifo := syscall.Mkfifo(pipePath, 0600); errFifo != nil {
		t.Fatal(errFifo)
	}
	defer os.Remove(pipePath)

	pipeFile, errOpen := os.OpenFile(pipePath, os.O_RDWR, os.ModeNamedPipe)
	if errOpen != nil {
		t.Fatal(errOpen)
	}
	defer pipeFile.Close()

	queryChan := make(chan *Query, 1)
	go WatchLogs(ctx, pipePath, queryChan, nil)

	if _, errWrite1 := pipeFile.WriteString(exampleLog1); errWrite1 != nil {
		t.Fatal(errWrite1)
	}
	query1 := <-queryChan
	equal(t, "google.com", query1.Domain)
	equal(t, "127.0.0.1", query1.MadeBy)
	equal(t, []string{"1.1.1.1"}, query1.Queried)
	equal(t, []string{"8.8.8.8"}, query1.Result)

	if _, errWrite2 := pipeFile.WriteString(exampleLog2); errWrite2 != nil {
		t.Fatal(errWrite2)
	}
	query2 := <-queryChan
	equal(t, "cf-dns", query2.Domain)
	equal(t, "192.168.1.1", query2.MadeBy)
	equal(t, 0, len(query2.Queried))
	equal(t, []string{"1.1.1.1", "1.0.0.1"}, query2.Result)

	cancel()
	<-queryChan
}
