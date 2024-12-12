# sk
Switch Kontext is a simple utility to quickly move between Kubernetes contexts and namespaces.

## Installation
Requires Go
``` bash
go install github.com/erikkinding/sk@v0.3.8
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
```

Primarily, sk looks at $KUBECONFIG to decide which configuration to use and alter. If not set, it defaults to ~/.kube/config. 


### Handy alias:
``` bash
alias skp="sk -p" # Previously selected context and namespace
alias skn="sk -n" # Also prompt for namespace selection
alias skN="sk -N" # Only prompt for namespace selection
alias skc="sk -c" # Print the currently selected context and namespace
alias skf="sk -f" # Jump to favorite
```
