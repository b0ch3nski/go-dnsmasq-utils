# go-dnsmasq-utils
[![license](https://img.shields.io/github/license/b0ch3nski/go-dnsmasq-utils)](LICENSE)
[![release](https://img.shields.io/github/v/release/b0ch3nski/go-dnsmasq-utils)](https://github.com/b0ch3nski/go-dnsmasq-utils/releases)
[![go.dev](https://pkg.go.dev/badge/github.com/b0ch3nski/go-dnsmasq-utils)](https://pkg.go.dev/github.com/b0ch3nski/go-dnsmasq-utils)
[![goreportcard](https://goreportcard.com/badge/github.com/b0ch3nski/go-dnsmasq-utils)](https://goreportcard.com/report/github.com/b0ch3nski/go-dnsmasq-utils)
[![issues](https://img.shields.io/github/issues/b0ch3nski/go-dnsmasq-utils)](https://github.com/b0ch3nski/go-dnsmasq-utils/issues)
[![sourcegraph](https://sourcegraph.com/github.com/b0ch3nski/go-dnsmasq-utils/-/badge.svg)](https://sourcegraph.com/github.com/b0ch3nski/go-dnsmasq-utils)

**DNSMasq** is basically everywhere, but has no easy to use API - let's (try to) fix that!

### dns queries

DNSMasq logs all DNS queries when `log-queries` flag is used. It also has an ability to write it's logs to syslog, file
and stderr. For a real time log streaming, named pipes can be used as a very basic method of IPC. Named pipe is a
blocking FIFO queue and therefore must be used with extra care (dnsmasq process will not start if there is no reader on
the other end of a pipe).

To gather all DNS queries resolved by DNSMasq, use following configuration options:
```
log-queries=extra
log-async=100
log-facility=/tmp/dnsmasq/log.fifo
```
Before starting dnsmasq, start `dnsmasq.WatchLogs(...)` function which will create Unix named pipe, parse all incoming
logs and publish resolved DNS queries over a Golang channel.

### dhcp leases

Reading DHCP leases doesn't require specific configuration changes. Leases file can be simply parsed using
`dnsmasq.ReadLeases(...)` function.

Additionally, function `dnsmasq.WatchLeases(...)` extends parsing with a Goroutine that reloads leases file when it was
modified (by listening on `IN_MODIFY` syscall signal) and sends all parsed leases over a channel.

## install

```
go get github.com/b0ch3nski/go-dnsmasq-utils
```

## example

```go
ctx := context.Background()

queries := make(chan *dnsmasq.Query)
go dnsmasq.WatchLogs(ctx, "/tmp/dnsmasq/log.fifo", queries, nil)

leases := make(chan []*dnsmasq.Lease)
go dnsmasq.WatchLeases(ctx, "/tmp/dnsmasq/dhcp.leases", leases)
```

Also see [docker-compose.yaml](docker-compose.yaml) file.
