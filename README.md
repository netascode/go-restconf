[![Tests](https://github.com/netascode/go-restconf/actions/workflows/test.yml/badge.svg)](https://github.com/netascode/go-restconf/actions/workflows/test.yml)

# go-restconf

`go-restconf` is a Go client library for RESTCONF devices. It is based on Nathan's excellent [goaci](https://github.com/brightpuddle/goaci) module and features a simple, extensible API and [advanced JSON manipulation](#result-manipulation).

## Getting Started

### Installing

To start using `go-restconf`, install Go and `go get`:

`$ go get -u github.com/netascode/go-restconf`

### Basic Usage

```go
package main

import "github.com/netascode/go-resconf"

func main() {
    client, _ := restconf.NewClient("https://1.1.1.1", "user", "pwd", true)

    res, _ := client.GetData("Cisco-IOS-XE-native:native")
    println(res.Res.Get("Cisco-IOS-XE-native:native.hostname").String())
}
```

This will print for example:

```
ROUTER-1
```

#### Result manipulation

`restconf.Result` uses GJSON to simplify handling JSON results. See the [GJSON](https://github.com/tidwall/gjson) documentation for more detail.

```go
res, _ := client.GetData("Cisco-IOS-XE-native:native/interface/GigabitEthernet")
println(res.Res.Get("Cisco-IOS-XE-native:GigabitEthernet.0.name").String()) // name of first interface

for _, int := range res.Res.Get("Cisco-IOS-XE-native:GigabitEthernet").Array() {
    println(int.Get("@pretty").Raw) // pretty print interface attributes
}
```

#### Helpers for common patterns

```go
res, _ := client.GetData("Cisco-IOS-XE-native:native/hostname")
res, _ := client.DeleteData("Cisco-IOS-XE-native:native/banner/login/banner")
```

#### Query parameters

Pass the `restconf.Query` object to the `Get` request to add query parameters:

```go
queryConfig := restconf.Query("content", "config")
res, _ := client.GetData("Cisco-IOS-XE-native:native", queryConfig)
```

Pass as many parameters as needed:

```go
res, _ := client.GetData("Cisco-IOS-XE-native:native",
    restconf.Query("content", "config"),
    restconf.Query("depth", "1"),
)
```

#### POST data creation

`restconf.Body` is a wrapper for [SJSON](https://github.com/tidwall/sjson). SJSON supports a path syntax simplifying JSON creation.

```go
exampleUser := restconf.Body{}.Set("Cisco-IOS-XE-native:username.name", "test-user").Str
client.PostData("Cisco-IOS-XE-native:native", exampleUser)
```

These can be chained:

```go
user1 := restconf.Body{}.
    Set("Cisco-IOS-XE-native:username.name", "test-user").
    Set("Cisco-IOS-XE-native:username.description", "My Test User")
```

...or nested:

```go
attrs := restconf.Body{}.
    Set("name", "test-user").
    Set("description", "My Test User").
    Str
user1 := restconf.Body{}.SetRaw("Cisco-IOS-XE-native:username", attrs).Str
```

## Documentation

See the [documentation](https://godoc.org/github.com/netascode/go-restconf) for more details.
