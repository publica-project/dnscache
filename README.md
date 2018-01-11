# DNS Lookup Cache

[![godoc](http://img.shields.io/badge/godoc-reference-blue.svg?style=flat)](https://godoc.org/github.com/rs/dnscache) [![license](http://img.shields.io/badge/license-MIT-red.svg?style=flat)](https://raw.githubusercontent.com/rs/dnscache/master/LICENSE) [![Build Status](https://travis-ci.org/rs/dnscache.svg?branch=master)](https://travis-ci.org/rs/dnscache) [![Coverage](http://gocover.io/_badge/github.com/rs/dnscache)](http://gocover.io/github.com/rs/dnscache)

The dnscache package provides a DNS cache layer to Go's `net.Resolver`.

# Install

Install using the "go get" command:

```
go get github.com/rs/dnscache
```

# Usage

Create a new instance and use it in place of `net.Resolver`. New names will be cached. Call the `Refresh` method at regular interval to update cached entries and cleanup unused ones.

```go
resolver := &dnscache.Resolver{}

// First call will cache the result
addrs, err := resolver.LookupHost("example.com")

// Subsequent calls will use the cached result
addrs, err = resolver.LookupHost("example.com")

// Call to refresh will refresh names in cache. If you pass true, it will also
// remove cached names not looked up since the last call to Refresh. It is a good idea
// to call this method on a regular interval.
go func() {
    clearUnused := true
    t := time.NewTicker(5 * time.Minute)
    defer t.Stop()
    for range t.C {
        resolver.Refresh(clearUnused)
    }
}()
```

If you are using an `http.Transport`, you can use this cache by specifying a `Dial` function:

```go
transport := &http.Transport {
  Dial: func(network string, address string) (net.Conn, error) {
    separator := strings.LastIndex(address, ":")
    ip, err := dnscache.LookupHost(address[:separator])
    if err != nil {
        return nil, err
    }
    return net.Dial("tcp", ip + address[separator:])
  },
}
```

