
## 0.1.12

- Extend wait time for release of device database lock to 90 seconds

## 0.1.11

- Honor proxy settings (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` environment variables)
- Add `Wait` request option to block until a potential database lock on the device has been released

## 0.1.10

- Improve handling of YANG-Patch errors

## 0.1.9

- Allow any type to be used with `Body.Set()`

## 0.1.8

- Add option to skip discovery
- Return error if discovery fails

## 0.1.7

- Fix error parsing if namespace is included in response

## 0.1.6

- Consider intermittently "inconsistent value" IOS-XE responses as transient errors

## 0.1.5

- Fix transient error handling

## 0.1.4

- Discover RESTCONF capabilities
- Add support for YANG-Patch (RFC 8072)

## 0.1.3

- Optimize retries

## 0.1.2

- Parse RESTCONF errors and add to response
- Improve error handling

## 0.1.1

- Return HTTP status code

## 0.1.0

- Initial release
