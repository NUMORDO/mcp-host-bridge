package bridge

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

var errOutputLimit = errors.New("backend output limit exceeded")

// lineBound admits only complete, bounded JSON lines before the SDK decoder.
// Checking validity per line prevents multiline JSON from bypassing the frame cap.
type lineBound struct {
	io.ReadCloser
	limit   int64
	failed  bool
	reader  *bufio.Reader
	pending []byte
	eof     bool
}

func (b *lineBound) Read(p []byte) (int, error) {
	if b.failed {
		return 0, errOutputLimit
	}
	if len(p) == 0 {
		return 0, nil
	}
	if len(b.pending) == 0 {
		if b.eof {
			return 0, io.EOF
		}
		if b.reader == nil {
			b.reader = bufio.NewReaderSize(b.ReadCloser, 4096)
		}
		var frame []byte
		for {
			chunk, err := b.reader.ReadSlice('\n')
			if int64(len(frame)+len(chunk)) > b.limit {
				b.failed = true
				return 0, errOutputLimit
			}
			frame = append(frame, chunk...)
			if err == bufio.ErrBufferFull {
				continue
			}
			if err != nil && err != io.EOF {
				b.failed = true
				return 0, err
			}
			if err == io.EOF {
				b.eof = true
				if len(frame) == 0 {
					return 0, io.EOF
				}
			}
			if !json.Valid(frame) {
				b.failed = true
				return 0, errors.New("backend must emit one complete JSON value per line")
			}
			b.pending = frame
			break
		}
	}
	n := copy(p, b.pending)
	b.pending = b.pending[n:]
	return n, nil
}

type totalBound struct {
	io.ReadCloser
	remaining int64
	failed    bool
}

func (b *totalBound) Read(p []byte) (int, error) {
	if b.failed {
		return 0, errOutputLimit
	}
	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1]
	}
	n, e := b.ReadCloser.Read(p)
	if int64(n) > b.remaining {
		valid := int(b.remaining)
		b.remaining = 0
		b.failed = true
		return valid, errOutputLimit
	}
	b.remaining -= int64(n)
	return n, e
}

type boundedHTTP struct {
	base  http.RoundTripper
	limit int64
}

func (b boundedHTTP) RoundTrip(r *http.Request) (*http.Response, error) {
	response, e := b.base.RoundTrip(r)
	if e != nil {
		return nil, e
	}
	if response.ContentLength > b.limit {
		response.Body.Close()
		return nil, errOutputLimit
	}
	response.Body = &totalBound{ReadCloser: response.Body, remaining: b.limit}
	return response, nil
}
