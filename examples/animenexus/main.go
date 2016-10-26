package main

import (
	"fmt"
	"strings"

	"github.com/noisypixy/go-icy"
)

const streamURLStr = "http://radio.animenexus.mx:8100/;stream.nsv"

func main() {
	s, err := icy.NewStream(streamURLStr)
	if err != nil {
		panic(err)
	}

	if err := s.Open(); err != nil {
		panic(err)
	}

	for {
		select {
		case metadata := <-s.Metadata():
			title := strings.TrimSpace(metadata.StreamTitle)
			if title != "" {
				fmt.Println(title)
			}
		}
	}

	if err := s.Close(); err != nil {
		panic(err)
	}
}
