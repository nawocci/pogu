package control

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/nawocci/pogu/internal/service"
	"github.com/nawocci/pogu/internal/telemetry"
)

const MaxLineSize = 1 << 20

const requestTimeout = 30 * time.Second

type Server struct {
	Service   *service.Service
	Telemetry *telemetry.Recorder
	tester    ProviderTestFunc

	mu       sync.Mutex
	listener *net.UnixListener
	path     string
	conns    map[net.Conn]struct{}
	closed   bool
}

func New(s *service.Service, tester ...any) *Server {
	v := &Server{Service: s, conns: make(map[net.Conn]struct{})}
	if len(tester) != 0 && tester[0] != nil {
		switch t := tester[0].(type) {
		case ProviderTestFunc:
			v.tester = t
		case func(context.Context, service.Provider, string) error:
			v.tester = ProviderTestFunc(t)
		case ProviderTester:
			v.tester = t.Test
		}
	}
	return v
}

func NewServer(s *service.Service, tester ...any) *Server { return New(s, tester...) }

func (s *Server) Start(path string) error {
	if s == nil || s.Service == nil {
		return errors.New("control: service is nil")
	}
	if path == "" {
		return errors.New("control: socket path is empty")
	}
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("control: refusing to replace non-socket %q", path)
		}
		probe, err := net.DialTimeout("unix", path, 200*time.Millisecond)
		if err == nil {
			_ = probe.Close()
			return errors.New("control: another daemon is already running")
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("control: remove stale socket: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("control: inspect socket: %w", err)
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return fmt.Errorf("control: listen: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(path)
		return fmt.Errorf("control: secure socket: %w", err)
	}
	s.mu.Lock()
	if s.listener != nil && !s.closed {
		s.mu.Unlock()
		_ = ln.Close()
		_ = os.Remove(path)
		return errors.New("control: server already started")
	}
	s.listener, s.path, s.closed = ln, path, false
	s.mu.Unlock()
	go s.acceptLoop(ln)
	return nil
}

func (s *Server) acceptLoop(ln *net.UnixListener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed || errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()
		go s.serveConn(conn)
	}
}

func (s *Server) serveConn(conn net.Conn) {
	defer func() {
		_ = conn.Close()
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
	}()
	reader := bufio.NewReaderSize(conn, 4096)
	enc := json.NewEncoder(conn)
	for {
		line, err := readLine(reader, MaxLineSize)
		if err != nil {
			if errors.Is(err, errLineTooLarge) {
				_ = enc.Encode(failure("request_too_large", "request exceeds size limit"))
			}
			return
		}
		if len(line) == 0 {
			continue
		}
		if err := enc.Encode(s.dispatch(line)); err != nil {
			return
		}
	}
}

var errLineTooLarge = errors.New("control: line too large")

func readLine(r *bufio.Reader, max int) ([]byte, error) {
	var line []byte
	for {
		part, err := r.ReadSlice('\n')
		if len(line)+len(part) > max {
			return nil, errLineTooLarge
		}
		line = append(line, part...)
		if err == nil {
			return bytes.TrimSuffix(line, []byte{'\n'}), nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return nil, err
		}
	}
}

func (s *Server) dispatch(line []byte) Response {
	var req Request
	if len(line) == 0 {
		return failure("invalid_request", "request must be a JSON object")
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	if err := dec.Decode(&req); err != nil {
		return failure("invalid_request", "invalid JSON request")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return failure("invalid_request", "request must contain one JSON value")
	}
	if req.Op == "" {
		return failure("invalid_request", "operation is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	result, err := s.execute(ctx, req)
	if err != nil {
		return failureFor(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return failure("internal", "internal control-plane error")
	}
	return Response{OK: true, Result: encoded}
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.listener == nil || s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	ln, path := s.listener, s.path
	s.listener, s.path = nil, ""
	conns := make([]net.Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	err := ln.Close()
	for _, c := range conns {
		_ = c.Close()
	}
	if e := os.Remove(path); err == nil || !errors.Is(e, os.ErrNotExist) {
		if err == nil {
			err = e
		}
	}
	return err
}
