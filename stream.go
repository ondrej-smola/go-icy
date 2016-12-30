package icy

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/url"
	"runtime"
	"strconv"
	"strings"
)

var (
	ErrUnexpectedEOS = errors.New("unexpected end of stream")
)

type state int

const (
	stateHeaders state = iota
	stateStream
	stateMetadata
)

type Stream interface {
	Open() error
	Close() error

	Headers() <-chan Headers
	Chunks() <-chan []byte
	Metadata() <-chan *Metadata
	Errors() <-chan error
}

type stream struct {
	urlStr    string
	conn      net.Conn
	interrupt chan struct{}

	chErrors   chan error
	chHeaders  chan Headers
	chChunks   chan []byte
	chMetadata chan *Metadata

	state     state
	chunkSize int

	splitFunc bufio.SplitFunc
}

func (s *stream) SplitFunc() bufio.SplitFunc {
	if s.splitFunc != nil {
		return s.splitFunc
	}

	s.splitFunc = func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		switch s.state {
		case stateHeaders:
			if atEOF == true {
				token = data
				advance = len(token)
				err = ErrUnexpectedEOS
				return
			}
			if !bytes.Contains(data, []byte{0x0d, 0x0a, 0x0d, 0x0a}) {
				return
			}

			stop := bytes.Index(data, []byte{0x0d, 0x0a, 0x0d, 0x0a})

			token = data[:stop+4]
			advance = len(token)

			s.state = stateStream

			err = s.handleHeaders(token)

			return

		case stateStream:
			if s.chunkSize <= 0 {
				err = errors.New("unknown chunk size")
				return
			}
			if len(data) < s.chunkSize {
				if atEOF {
					advance = len(data)
					token = data
					err = ErrUnexpectedEOS
				}
				return
			}

			token = data[:s.chunkSize]
			advance = len(token)

			s.state = stateMetadata

			err = s.handleChunk(token)

			return

		case stateMetadata:
			metadataLength := int(data[0]*16) + 1
			if len(data) < metadataLength {
				return
			}

			token = data[:metadataLength]
			advance = len(token)

			s.state = stateStream

			err = s.handleMetadata(token[1:])

			return
		}

		return
	}

	return s.splitFunc
}

func (s *stream) Open() (err error) {
	var (
		streamURL *url.URL
		conn      net.Conn
	)

	streamURL, err = url.Parse(s.urlStr)
	if err != nil {
		return
	}

	conn, err = net.Dial("tcp", streamURL.Host)
	if err != nil {
		return
	}

	s.conn = conn
	s.state = stateHeaders

	fmt.Fprintf(s.conn, "GET %s HTTP/1.0\r\n", streamURL.Path)
	fmt.Fprintf(s.conn, "Icy-MetaData:1\r\n")
	fmt.Fprintf(s.conn, "\r\n")

	s.interrupt = make(chan struct{})

	interrupted := false

	go func() {
		<-s.interrupt

		interrupted = true
	}()

	go func() {
		r := bufio.NewReader(conn)

		scanner := bufio.NewScanner(r)
		scanner.Split(s.SplitFunc())

		for !interrupted && scanner.Scan() {
			runtime.Gosched()
		}
		if err := scanner.Err(); err != nil {
			if s.chErrors == nil {
				panic(err)
			} else {
				s.chErrors <- err
			}
			return
		}
	}()

	return
}

func (s *stream) Close() (err error) {
	close(s.interrupt)

	return
}

func (s *stream) Errors() <-chan error {
	if s.chErrors == nil {
		s.chErrors = make(chan error)
	}

	return s.chErrors
}

func (s *stream) Headers() <-chan Headers {
	if s.chHeaders == nil {
		s.chHeaders = make(chan Headers)
	}

	return s.chHeaders
}

func (s *stream) Chunks() <-chan []byte {
	if s.chChunks == nil {
		s.chChunks = make(chan []byte)
	}

	return s.chChunks
}

func (s *stream) Metadata() <-chan *Metadata {
	if s.chMetadata == nil {
		s.chMetadata = make(chan *Metadata)
	}

	return s.chMetadata
}

func (s *stream) handleHeaders(rawHeaders []byte) (err error) {
	headers := make(Headers, 0)

	rawHeadersStr := string(rawHeaders)
	rawHeadersStr = strings.TrimSpace(rawHeadersStr)

	headerLines := strings.Split(rawHeadersStr, "\n")

	for i, line := range headerLines {
		line = strings.TrimSpace(line)

		if i == 0 {
			if line != "ICY 200 OK" {
				err = errors.New("unexpected response status")
				return
			}
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			err = errors.New("invalid headers")
			return
		}

		key := parts[0]
		value := parts[1]

		headers.Add(key, value)
	}

	if s.chHeaders != nil {
		s.chHeaders <- headers
	}

	chunkSizeStr := headers.Get("icy-metaint")

	s.chunkSize, err = strconv.Atoi(chunkSizeStr)
	if err != nil {
		return
	}

	return
}

func (s *stream) handleChunk(rawChunk []byte) (err error) {
	if s.chChunks == nil {
		return
	}

	s.chChunks <- rawChunk

	return
}

func (s *stream) handleMetadata(rawMetadata []byte) (err error) {
	if s.chMetadata == nil {
		return
	}
	if len(rawMetadata) == 0 {
		return
	}

	var (
		metadata *Metadata
	)

	rawMetadata = bytes.TrimRight(rawMetadata, "\x00")

	rawMetadataStr := string(rawMetadata)

	metadata, err = ParseMetadata(rawMetadataStr)
	if err != nil {
		return
	}

	s.chMetadata <- metadata

	return
}

func ParseMetadata(metadataStr string) (metadata *Metadata, err error) {
	if metadataStr == "" {
		return
	}

	metadata = &Metadata{}

	tokens := strings.Split(metadataStr, "';")
	for _, token := range tokens {
		if token == "" {
			break
		}

		parts := strings.SplitN(token, "='", 2)
		if len(parts) != 2 {
			err = errors.New("invalid token")
			return
		}

		key := parts[0]
		value := parts[1]

		switch key {
		case "StreamTitle":
			metadata.StreamTitle = value
			break
		}
	}

	return
}

func NewStream(urlStr string) (s Stream, err error) {
	if urlStr == "" {
		err = errors.New("empty urlStr")
		return
	}

	s = &stream{
		urlStr: urlStr,
	}

	return
}
