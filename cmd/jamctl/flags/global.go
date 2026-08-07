package flags

import (
	"github.com/spf13/pflag"
)

type GlobalFlags struct {
	Verbose bool
}

func NewGlobalFlags() *GlobalFlags {
	return &GlobalFlags{}
}

func (g *GlobalFlags) AttachFlags(flags *pflag.FlagSet) {
	flags.BoolVarP(&g.Verbose, "verbose", "v", true, "Enable verbose outputs")
}
