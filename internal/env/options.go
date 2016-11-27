package env

func (o *option) Name() string {
	return o.name
}

func (o *option) Value() interface {} {
	return o.value
}

// WithLoadEnvdir specifies if Loader should load the original
// environment variables AND the contents of envdir
func WithLoadEnvdir(b bool) Option {
	return &option{
		name: LoadEnvdirKey,
		value: b,
	}
}
