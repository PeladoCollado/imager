package requests

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/PeladoCollado/imager/types"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
)

// StreamReader is an instance of RequestSource that iterates over a list of JSON requests, using a static scheme and
// host. The JSON records are decoded as Request instances. As the Requests returned by Next() are decoded from a
// io.Reader, the response body is never read in the Read(*http.Response) method.
type StreamReader struct {
	scheme  string
	host    string
	decoder *json.Decoder
	r       io.Reader
}

type Request struct {
	Method      string
	Path        string
	QueryString string
	Headers     map[string][]string
	Body        string
}

func NewFileReader(file string, scheme string, host string) (types.RequestSource, error) {
	fh, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(fh)
	return &StreamReader{
		scheme:  scheme,
		host:    host,
		decoder: decoder,
		r:       fh,
	}, nil
}

func (s *StreamReader) Next() (*http.Request, error) {
	next := &Request{}
	err := s.decoder.Decode(next)
	if errors.Is(err, io.EOF) {
		err = s.Reset()
		if err == nil {
			return nil, err
		} else {
			return s.Next()
		}
	}
	url := &url.URL{
		Scheme:   s.scheme,
		Host:     s.host,
		Path:     next.Path,
		RawQuery: next.QueryString,
	}
	var body io.ReadCloser
	if next.Body != "" {
		body = ioutil.NopCloser(bytes.NewBufferString(next.Body))
	}
	return &http.Request{
		Method: next.Method,
		URL:    url,
		Header: next.Headers,
		Body:   body,
	}, nil
}

func (s *StreamReader) Reset() error {
	if seeker, ok := s.r.(io.Seeker); ok {
		_, err := seeker.Seek(0, io.SeekStart)
		return err
	} else {
		return io.EOF
	}
}

func (s *StreamReader) Read(resp *http.Response) (int64, error) {
	// we don't care about the response here
	return io.Copy(ioutil.Discard, resp.Body)
}
