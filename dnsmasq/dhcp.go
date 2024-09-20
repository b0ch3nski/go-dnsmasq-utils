package dnsmasq

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/netip"
	"os"
	"regexp"
	"strconv"
	"syscall"
	"time"
)

// for fields descriptions, see: https://narkive.com/goLCUHEc
var rgxLease = regexp.MustCompile(`(\d{10}|0) (\S+) (\S+) (\S+) \S+`)

// Lease is the representation of DHCP lease from DNSMasq.
type Lease struct {
	Expires  time.Time
	MacAddr  net.HardwareAddr
	IPAddr   netip.Addr
	Hostname string
}

// WatchLeases sends all read DHCP leases over `output` channel every time lease file at `filePath` gets updated.
// Function is blocking until context is done.
func WatchLeases(ctx context.Context, filePath string, output chan<- []*Lease) error {
	defer close(output)

	leaseFile, errOpen := os.OpenFile(filePath, os.O_RDONLY|os.O_CREATE, 0644)
	if errOpen != nil {
		return errOpen
	}
	defer leaseFile.Close()

	reloadSig, errWatch := watchFileChanges(ctx, filePath)
	if errWatch != nil {
		return errWatch
	}

	for range reloadSig {
		// rewind lease file to the beginning
		if _, errSeek := leaseFile.Seek(0, io.SeekStart); errSeek != nil {
			return errSeek
		}

		leases, errRead := ReadLeases(leaseFile)
		if len(leases) > 0 {
			output <- leases
		}
		if errRead != nil {
			return errRead
		}
	}

	return nil
}

// ReadLeases reads DHCP leases from file using provided `io.Reader`.
func ReadLeases(reader io.Reader) ([]*Lease, error) {
	leases := make([]*Lease, 0)
	bufRd := bufio.NewReader(reader)

	for {
		line, errRead := bufRd.ReadString('\n')
		if errRead != nil {
			if errRead == io.EOF {
				break
			}
			return leases, errRead
		}

		if matchLease := rgxLease.FindStringSubmatch(line); matchLease != nil {
			expInt, errExpInt := strconv.ParseInt(matchLease[1], 10, 64)
			if errExpInt != nil {
				return leases, errExpInt
			}
			var exp time.Time
			if expInt != 0 {
				exp = time.Unix(expInt, 0)
			}

			mac, errMac := net.ParseMAC(matchLease[2])
			if errMac != nil {
				return leases, errMac
			}
			ip, errIp := netip.ParseAddr(matchLease[3])
			if errIp != nil {
				return leases, errIp
			}

			leases = append(leases, &Lease{Expires: exp, MacAddr: mac, IPAddr: ip, Hostname: matchLease[4]})
		}
	}

	return leases, nil
}

func watchFileChanges(ctx context.Context, filePath string) (<-chan struct{}, error) {
	fileDesc, errInit := syscall.InotifyInit1(syscall.IN_CLOEXEC)
	if errInit != nil {
		return nil, errInit
	}

	watch, errWatch := syscall.InotifyAddWatch(fileDesc, filePath, syscall.IN_MODIFY)
	if errWatch != nil {
		return nil, errWatch
	}

	go func() {
		<-ctx.Done()
		syscall.InotifyRmWatch(fileDesc, uint32(watch))
		syscall.Close(fileDesc)
	}()

	buf := make([]byte, syscall.SizeofInotifyEvent)
	ch := make(chan struct{}, 1)

	go func() {
		defer close(ch)

		for {
			if n, _ := syscall.Read(fileDesc, buf[:]); n == syscall.SizeofInotifyEvent {
				if ctx.Err() != nil {
					return
				}
				ch <- struct{}{}
			}
		}
	}()

	return ch, nil
}
