# sk
Switch Kontext is a simple utility to quickly move between Kubernetes contexts and namespaces.

## Installation

### Homebrew (macOS/Linux)
``` bash
brew tap erikkinding/tap
brew install erikkinding/tap/sk
```

### Go
Requires Go
``` bash
go install github.com/erikkinding/sk@v0.4.2
```
Make sure your Go bin directory is part of your path, or create an alias for `sk`.

## Usage
``` bash
sk -h

Output:
  -F string
        Store current context and namespace as favorite
  -N    Only select namespace from the ones available for the selected context
  -c    Print the currently selected context and namespace
  -f string
        Select a favorite context
  -l    List all stored favorites
  -n    Select namespace from the ones available for the selected context
  -p    Use to switch to the previously used context and namespace. Has no effect if state can't be retrieved.
  -v    Print the current version
  -     Shorthand for -p. (Yes, just a lonely dash)
```

Primarily, sk looks at $KUBECONFIG to decide which configuration to use and alter. If not set, it defaults to ~/.kube/config.

### Examples

**Switch context (interactive prompt):**
``` bash
sk
# Presents a list of available contexts to pick from.
# Selected context becomes active immediately.
```

**Switch context and pick a namespace in one go:**
``` bash
sk -n
# First prompts for a context, then prompts for a namespace within that context.
```

**Switch namespace only (stay in the current context):**
``` bash
sk -N
# Useful when you're already in the right cluster but need a different namespace.
```

**Jump back to the previous context and namespace:**
``` bash
sk -
# or: sk -p
# Handy for toggling between two clusters, e.g. staging ↔ production.
```

**Save the current context/namespace as a favorite:**
``` bash
sk -F prod-eu
# Stores the active context and namespace under the alias "prod-eu".
```

**Jump directly to a saved favorite:**
``` bash
sk -f prod-eu
# Switches context and namespace in one command, no prompts.
```

**List all saved favorites:**
``` bash
sk -l
```

**Check what context and namespace is currently active:**
``` bash
sk -c
# Prints e.g. "context: prod-eu-1 | namespace: payments"
```


### Handy alias:
``` bash
alias skp="sk -p" # Previously selected context and namespace
alias skn="sk -n" # Also prompt for namespace selection
alias skN="sk -N" # Only prompt for namespace selection
alias skc="sk -c" # Print the currently selected context and namespace
alias skf="sk -f" # Jump to favorite
```

## Development

### Unit tests
Run the lightweight unit tests (no external dependencies):
``` bash
go test -v .
```

### Integration tests
Integration tests spin up a real Kubernetes cluster inside a Docker container using
[k3s via testcontainers-go](https://golang.testcontainers.org/modules/k3s/).  
Requires a running Docker daemon.

``` bash
go test -tags integration -v -timeout 5m .
```

The test suite (`integration_test.go`) covers every feature end-to-end:
- listing and switching contexts
- listing namespaces from the live cluster and switching namespaces
- storing / loading / listing favorites (`-F`, `-f`, `-l`)
- previous-context restore (`-p`)
- printing the current context and namespace (`-c`)
- selection validation

