package signprocessor

type Config struct {
	KeyFile string `mapstructure:"key_file"`
}

func (c *Config) Validate() error {
	if c.KeyFile == "" {
		return configError("key_file is required")
	}
	return nil
}

type configError string

func (e configError) Error() string { return string(e) }
