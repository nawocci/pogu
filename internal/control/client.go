package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
)

type Client struct {
	Path        string
	MaxLineSize int
}

func NewClient(path string) *Client { return &Client{Path: path, MaxLineSize: MaxLineSize} }

func Dial(path string) *Client { return NewClient(path) }

func (c *Client) Do(ctx context.Context, req Request) (Response, error) {
	if c == nil || c.Path == "" {
		return Response{}, errors.New("control: client socket path is empty")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.Path)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	line, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	limit := c.MaxLineSize
	if limit <= 0 {
		limit = MaxLineSize
	}
	if len(line) > limit {
		return Response{}, errors.New("control: request exceeds size limit")
	}
	if _, err = conn.Write(append(line, '\n')); err != nil {
		return Response{}, err
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 4096), limit)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return Response{}, err
		}
		return Response{}, io.EOF
	}
	var resp Response
	if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
		return Response{}, errors.New("control: invalid response")
	}
	if !resp.OK && resp.Error != nil {
		return resp, &RemoteError{Code: resp.Error.Code, Message: resp.Error.Message}
	}
	return resp, nil
}

func (c *Client) Call(ctx context.Context, req Request, out any) error {
	resp, err := c.Do(ctx, req)
	if err != nil {
		return err
	}
	if out == nil || len(resp.Result) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Result, out)
}

type RemoteError struct{ Code, Message string }

func (e *RemoteError) Error() string { return e.Code + ": " + e.Message }
