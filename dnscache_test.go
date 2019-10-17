package dnscache

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestResolver_LookupHost(t *testing.T) {
	r := NewDNSResolver(128)
	var cacheMiss bool
	r.OnCacheMiss = func() {
		cacheMiss = true
	}
	hosts := []string{"google.com", "google.com.", "netflix.com"}
	for _, host := range hosts {
		t.Run(host, func(t *testing.T) {
			for _, wantMiss := range []bool{true, false, false} {
				cacheMiss = false
				addrs, err := r.LookupHost(context.Background(), host)
				if err != nil {
					t.Fatal(err)
				}
				if len(addrs) == 0 {
					t.Error("got no record")
				}
				for _, addr := range addrs {
					if net.ParseIP(addr) == nil {
						t.Errorf("got %q; want a literal IP address", addr)
					}
				}
				if wantMiss != cacheMiss {
					t.Errorf("got cache miss=%v, want %v", cacheMiss, wantMiss)
				}
			}
		})
	}
}

func TestRaceOnDelete(t *testing.T) {
	r := NewDNSResolver(128)
	ls := make(chan bool)
	rs := make(chan bool)

	go func() {
		for {
			select {
			case <-ls:
				return
			default:
				r.LookupHost(context.Background(), "google.com")
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-rs:
				return
			default:
				r.Refresh()
				time.Sleep(time.Millisecond)
			}
		}
	}()

	time.Sleep(1 * time.Second)

	ls <- true
	rs <- true

}
