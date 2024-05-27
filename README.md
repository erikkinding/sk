# sk
Switch Kontext is a simple utility to quickly move between Kubernetes contexts and namespaces.

## Installation
``` bash
# Requires Go
go install github.com/erikkinding/sk@v0.3.2
```
Make sure your Go bin directory is part of your path, or create an alias for `sk`.

## Usage
``` bash
sk -h

Output:
-N    Only select namespace from the ones available for the selected context
-n    Select namespace from the ones available for the selected context
-p    Use to switch to the previously used context and namespace. Has no effect if state can't be retrieved from temp file.
```

Primarily, sk looks at $KUBECONFIG to decide which configuration to use and alter. If not set, it defaults to ~/.kube/config. 


### Handy alias:
``` bash
alias skp="sk -p" # Previously selected context and namespace
alias skn="sk -n" # Also prompt for namespace selection
alias skN="sk -N" # Only prompt for namespace selection
alias skc="sk -c" # Print the currently selected context and namespace
```
