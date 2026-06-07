package main

import (
	"embed"
	"log"

	"ignore/internal/app"
	"ignore/internal/ui"
)

//go:embed all:ui/dist
var assets embed.FS

func main() {
	ignore, err := app.New(assets)
	if err != nil {
		log.Fatal(err)
	}
	if err := ui.Run(ignore); err != nil {
		log.Fatal(err)
	}
}
