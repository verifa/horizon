package terra

type Outputer interface {
	// Name is the name of the output.
	// It must be unique across all the outputs.
	Name() string
	// Resource is the resource that this output references.
	Resource() string
}
