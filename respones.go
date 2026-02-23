package nacos

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Response struct {
	*http.Response
	httpErr error
}

func (r *Response) CheckStatus() error {
	if r.StatusCode != http.StatusOK {
		data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			return fmt.Errorf("%s: %s %w", r.Status, r.Request.URL, err)
		}
		if data[0] == '<' {
			return fmt.Errorf("%s: %s", r.Status, r.Request.URL)
		}
		return fmt.Errorf("%s: %s %s", r.Status, r.Request.URL, data)
	}
	return nil
}

func (r *Response) CheckError() error {
	if r.httpErr != nil {
		return r.httpErr
	}
	defer r.Body.Close()
	return r.CheckStatus()
}

func (r *Response) Decode(v any) error {
	if r.httpErr != nil {
		return r.httpErr
	}
	defer r.Body.Close()
	if err := r.CheckStatus(); err != nil {
		return err
	}
	return json.NewDecoder(r.Body).Decode(v)
}
