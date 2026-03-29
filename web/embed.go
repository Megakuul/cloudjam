package web

import (
	"embed"
	_ "embed"
)

//go:embed build/**
var Files embed.FS
