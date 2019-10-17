package dnscache

import (
	"context"
	"net"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"

	"golang.org/x/sync/singleflight"
)

type DNSResolver interface {
	LookupHost(ctx context.Context, host string) (addrs []string, err error)
	LookupAddr(ctx context.Context, addr string) (names []string, err error)
}

type Resolver struct {
	// Timeout defines the maximum allowed time allowed for a lookup.
	Timeout time.Duration

	// Resolver is used to perform actual DNS lookup. If nil,
	// net.DefaultResolver is used instead.
	Resolver DNSResolver

	once  sync.Once
	mu    sync.RWMutex
	cache *lru.Cache

	// OnCacheMiss is executed if the host or address is not included in
	// the cache and the default lookup is executed.
	OnCacheMiss func()
}

type cacheEntry struct {
	rrs []string
	err error
}

func NewDNSResolver(cacheSize int) *Resolver {
	cache, _ := lru.New(cacheSize)
	return &Resolver{
		cache: cache,
	}
}

// LookupAddr performs a reverse lookup for the given address, returning a list
// of names mapping to that address.
func (r *Resolver) LookupAddr(ctx context.Context, addr string) (names []string, err error) {
	return r.lookup(ctx, "r"+addr)
}

// LookupHost looks up the given host using the local resolver. It returns a
// slice of that host's addresses.
func (r *Resolver) LookupHost(ctx context.Context, host string) (addrs []string, err error) {
	return r.lookup(ctx, "h"+host)
}

// Refresh refreshes all cached entries
func (r *Resolver) Refresh() {
	for _, key := range r.cache.Keys() {
		r.update(context.Background(), key.(string))
	}
}

// lookupGroup merges lookup calls together for lookups for the same host. The
// lookupGroup key is is the LookupIPAddr.host argument.
var lookupGroup singleflight.Group

func (r *Resolver) lookup(ctx context.Context, key string) (rrs []string, err error) {
	var found bool
	rrs, found, err = r.load(key)
	if !found {
		if r.OnCacheMiss != nil {
			r.OnCacheMiss()
		}
		rrs, err = r.update(ctx, key)
	}
	return
}

func (r *Resolver) update(ctx context.Context, key string) (rrs []string, err error) {
	c := lookupGroup.DoChan(key, r.lookupFunc(key))
	select {
	case <-ctx.Done():
		err = ctx.Err()
		if err == context.DeadlineExceeded {
			// If DNS request timed out for some reason, force future
			// request to start the DNS lookup again rather than waiting
			// for the current lookup to complete.
			lookupGroup.Forget(key)
		}
	case res := <-c:
		if res.Shared {
			// We had concurrent lookups, check if the cache is already updated
			// by a friend.
			var found bool
			rrs, found, err = r.load(key)
			if found {
				return
			}
		}
		err = res.Err
		if err == nil {
			rrs, _ = res.Val.([]string)
		}
		r.mu.Lock()
		r.storeLocked(key, rrs, err)
		r.mu.Unlock()
	}
	return
}

// lookupFunc returns lookup function for key. The type of the key is stored as
// the first char and the lookup subject is the rest of the key.
func (r *Resolver) lookupFunc(key string) func() (interface{}, error) {
	if len(key) == 0 {
		panic("lookupFunc with empty key")
	}

	var resolver DNSResolver = net.DefaultResolver
	if r.Resolver != nil {
		resolver = r.Resolver
	}

	switch key[0] {
	case 'h':
		return func() (interface{}, error) {
			ctx, cancel := r.getCtx()
			defer cancel()
			return resolver.LookupHost(ctx, key[1:])
		}
	case 'r':
		return func() (interface{}, error) {
			ctx, cancel := r.getCtx()
			defer cancel()
			return resolver.LookupAddr(ctx, key[1:])
		}
	default:
		panic("lookupFunc invalid key type: " + key)
	}
}

func (r *Resolver) getCtx() (ctx context.Context, cancel context.CancelFunc) {
	ctx = context.Background()
	if r.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
	} else {
		cancel = func() {}
	}
	return
}

func (r *Resolver) load(key string) (rrs []string, found bool, err error) {
	r.mu.RLock()
	entry, found := r.cache.Get(key)
	if !found {
		r.mu.RUnlock()
		return
	}
	rrs = entry.(*cacheEntry).rrs
	err = entry.(*cacheEntry).err
	r.mu.RUnlock()
	return rrs, true, err
}

func (r *Resolver) storeLocked(key string, rrs []string, err error) {
	if entry, found := r.cache.Get(key); found {
		// Update existing entry in place
		entry.(*cacheEntry).rrs = rrs
		entry.(*cacheEntry).err = err
		return
	}
	r.cache.Add(key, &cacheEntry{
		rrs: rrs,
		err: err,
	})
}
