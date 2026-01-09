package main

import "embed"

//go:embed all:web
var embeddedFiles embed.FS
