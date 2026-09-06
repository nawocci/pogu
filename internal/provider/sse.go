package provider

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
)

func scanSSE(ctx context.Context, r io.Reader, onEvent func(event, data string) bool) error {
	reader := bufio.NewReader(r)
	var event string
	var dataLines []string
	flush := func() bool {
		if len(dataLines) == 0 {
			event = ""
			return true
		}
		current, data := event, strings.Join(dataLines, "\n")
		event, dataLines = "", nil
		return onEvent(current, data)
	}
	buf := make([]byte, 32*1024)
	var pending []byte
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := reader.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
			for {
				idx := bytes.IndexByte(pending, '\n')
				if idx < 0 {
					break
				}
				line := strings.TrimSuffix(string(pending[:idx]), "\r")
				pending = pending[idx+1:]
				if line == "" {
					if !flush() {
						return context.Canceled
					}
					continue
				}
				if strings.HasPrefix(line, ":") {
					continue
				}
				if name, value, ok := strings.Cut(line, ":"); ok {
					value = strings.TrimPrefix(value, " ")
					switch name {
					case "event":
						event = value
					case "data":
						dataLines = append(dataLines, value)
					}
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				if len(dataLines) > 0 && !onEvent(event, strings.Join(dataLines, "\n")) {
					return context.Canceled
				}
				return nil
			}
			return &UpstreamError{Err: readErr}
		}
	}
}
