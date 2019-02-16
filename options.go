package gzip

var (
	DefaultExcludedExtentions = NewExcludedExtensions([]string{
		".png", ".gif", ".jpeg", ".jpg",
	})
	DefaultOptions = &Options{
		ExcludedExtensions: DefaultExcludedExtentions,
	}
)

type Options struct {
	ExcludedExtensions ExcludedExtensions
}

type Option func(*Options)

func WithExcludedExtensions(args []string) Option {
	return func(o *Options) {
		o.ExcludedExtensions = NewExcludedExtensions(args)
	}
}

// Using map for better lookup performance
type ExcludedExtensions map[string]bool

func NewExcludedExtensions(extensions []string) ExcludedExtensions {
	res := make(ExcludedExtensions)
	for _, e := range extensions {
		res[e] = true
	}
	return res
}

func (e ExcludedExtensions) Contains(target string) bool {
	_, ok := e[target]
	return ok
}
