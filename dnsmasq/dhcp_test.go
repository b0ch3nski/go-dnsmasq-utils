package dnsmasq

import (
	"context"
	"net"
	"net/netip"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

const exampleLease1 = `
1701658085 10:2b:41:04:88:95 192.168.1.3 some-host-name 01:10:2b:41:04:88:95
0 e8:fd:f8:33:4f:80 192.168.1.100 static-host-name 01:e8:fd:f8:33:4f:80
`

const exampleLease2 = `
123 garbage
1706997804 3d:14:49:d5:dd:f1 127.0.0.1 localhost 01:3d:14:49:d5:dd:f1
0123456789 yeah:D this looks valid
`

func testCommon(t *testing.T, leases []*Lease) {
	equal(t, 2, len(leases))
	equal(t, int64(1701658085), leases[0].Expires.Unix())
	equal(t, net.HardwareAddr{16, 43, 65, 4, 136, 149}, leases[0].MacAddr)
	equal(t, netip.AddrFrom4([4]byte{192, 168, 1, 3}), leases[0].IPAddr)
	equal(t, "some-host-name", leases[0].Hostname)
	equal(t, true, leases[1].Expires.IsZero())
}

func TestReadLeases(t *testing.T) {
	leases, errRead := ReadLeases(strings.NewReader(exampleLease1))
	if errRead != nil {
		t.Fatal(errRead)
	}
	testCommon(t, leases)
}

func TestWatchLeases(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	leaseFile, errCreate := os.CreateTemp("", "dhcp-leases-")
	if errCreate != nil {
		t.Fatal(errCreate)
	}
	defer os.Remove(leaseFile.Name())
	defer leaseFile.Close()

	leaseChan := make(chan []*Lease, 1)
	go WatchLeases(ctx, leaseFile.Name(), leaseChan)

	// give some time for syscall registration
	time.Sleep(100 * time.Millisecond)

	if _, errWrite1 := leaseFile.WriteString(exampleLease1); errWrite1 != nil {
		t.Fatal(errWrite1)
	}
	testCommon(t, <-leaseChan)

	if _, errWrite2 := leaseFile.WriteString(exampleLease2); errWrite2 != nil {
		t.Fatal(errWrite2)
	}
	leases := <-leaseChan
	equal(t, 3, len(leases))
	equal(t, "localhost", leases[2].Hostname)

	cancel()
	<-leaseChan
}

func equal(t *testing.T, expected, actual any) {
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("Not equal: \nexpected: %v\nactual  : %v", expected, actual)
	}
}
