# Stuart

Stuart is next generation event library and event driven
workflow engine written in Go.

## Stuart works as a library

```go
import (
	_ "github.com/gravitational/stuart"
)

// use it as a library:
func main() {
	Process() // ....
	ListenAndServe()
}
```

## As a CRD k8s resource

In this case CRD controller will compile the stuart program
and launch a handler as a separate controller.

```yaml
Kind: stuart.gravitational.com/v1
Spec: |
   Process()...
```
