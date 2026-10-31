package main

import (
	"log"
	"os"
	"os/signal"

	"github.com/egoist/mygo"
	_ "github.com/UruhaLushia/tomori/locales"
)

func main() {
	stopInterrupt := watchInterrupt()
	defer stopInterrupt()
	if err := Execute(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func watchInterrupt() func() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	done := make(chan struct{})
	go func() {
		select {
		case <-signals:
			mygo.App.Quit()
		case <-done:
		}
	}()
	return func() {
		signal.Stop(signals)
		close(done)
	}
}
