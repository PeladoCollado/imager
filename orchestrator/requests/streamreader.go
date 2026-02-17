package requests

import (
	"encoding/json"
	"errors"
	"github.com/PeladoCollado/imager/types"
	"io"
	"os"
)

// StreamReader is an instance of RequestSource that iterates over a list of JSON request specs.
type StreamReader struct {
	decoder *json.Decoder
	r       io.Reader
}

func NewFileReader(file string) (types.RequestSource, error) {
	fh, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(fh)
	return &StreamReader{
		decoder: decoder,
		r:       fh,
	}, nil
}

func (s *StreamReader) Next() (types.RequestSpec, error) {
	next := types.RequestSpec{}
	err := s.decoder.Decode(&next)
	if errors.Is(err, io.EOF) {
		err = s.Reset()
		if err != nil {
			return types.RequestSpec{}, err
		}
		return s.Next()
	}
	if err != nil {
		return types.RequestSpec{}, err
	}
	return next, nil
}

func (s *StreamReader) Reset() error {
	if seeker, ok := s.r.(io.Seeker); ok {
		_, err := seeker.Seek(0, io.SeekStart)
		return err
	} else {
		return io.EOF
	}
}
