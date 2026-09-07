package email

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTPServer is a minimal SMTP responder used by the
// Client tests. It accepts one connection at a time per instance
// and records the conversation so tests can assert on what was
// sent. Per-method behavior (fail on AUTH, MAIL, RCPT, or DATA)
// is controlled by the booleans on the struct so the same
// responder is reused across the happy path and every error
// branch.
//
// The server is plain TCP (no TLS) so the tests can inject a
// custom Dialer that returns a raw net.Conn. This isolates the
// SMTP-conversation logic under test from the TLS layer (which
// is stdlib crypto/tls and not something we need to verify in
// unit tests — a real Cloudflare integration test would be
// the right place for that, and is out of scope here).
type fakeSMTPServer struct {
	listener net.Listener
	mu       sync.Mutex
	messages []receivedMessage
	failOn   string // "auth" | "mail" | "rcpt" | "data" | "" for none
	closed   bool
}

type receivedMessage struct {
	from string
	to   string
	body []byte
}

// newFakeSMTPServer starts the server on a random localhost port.
// The returned cleanup func closes the listener; tests should
// defer it.
func newFakeSMTPServer(t *testing.T) (*fakeSMTPServer, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeSMTPServer{listener: ln}
	go s.acceptLoop()
	t.Cleanup(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		_ = ln.Close()
	})
	return s, ln.Addr().String()
}

func (s *fakeSMTPServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}
			// Transient accept error — sleep briefly and try
			// again; the listener is shared across the test
			// process so a racing Close is possible.
			time.Sleep(10 * time.Millisecond)
			continue
		}
		go s.handle(conn)
	}
}

// handle drives a single SMTP conversation. The state machine
// follows RFC 5321 §4.5: a 220 greeting, then EHLO, AUTH, MAIL,
// RCPT, DATA, and a clean QUIT. Each step looks at s.failOn to
// decide whether to send the success or failure reply.
func (s *fakeSMTPServer) handle(conn net.Conn) {
	defer conn.Close()
	rw := textproto.NewConn(conn)
	defer rw.Close()

	sendLine := func(code int, text string) {
		fmt.Fprintf(conn, "%d %s\r\n", code, text)
	}

	// Greet.
	sendLine(220, "fake smtp server ready")

	// We need to track the from/to across MAIL/RCPT for the
	// final DATA terminator and for the recorded message.
	var from, to string
	var bodyBuf strings.Builder

	for {
		line, err := rw.ReadLine()
		if err != nil {
			return
		}
		// Dot-stuffing reversal: SMTP doubles leading dots on
		// the wire; undo before recording the body.
		undot := func(l string) string { return strings.TrimPrefix(l, "..") }

		// The DATA terminator is a lone "." line, which we
		// process inside the DATA branch rather than the main
		// switch. So if we are "in data", the "." line is the
		// terminator; otherwise it's a command.
		cmd, args := splitCmd(line)

		switch cmd {
		case "EHLO", "HELO":
			// Multi-line response: 250- on every line except
			// the last, which uses 250 + space. A single-line
			// response is just "250 <text>\r\n". The Go
			// smtp client uses the AUTH advertisement to
			// decide whether to do an implicit single-round
			// AUTH PLAIN or a challenge/response dance, so
			// the prefix on the AUTH line is load-bearing.
			fmt.Fprintf(conn, "250-fake\r\n")
			fmt.Fprintf(conn, "250-AUTH PLAIN LOGIN\r\n")
			fmt.Fprintf(conn, "250 8BITMIME\r\n")
		case "AUTH":
			if s.failOn == "auth" {
				sendLine(535, "5.7.8 auth failed")
				continue
			}
			sendLine(235, "2.7.0 auth ok")
		case "MAIL":
			if s.failOn == "mail" {
				sendLine(550, "5.7.1 sender denied")
				continue
			}
			from = extractAddr(args)
			sendLine(250, "2.1.0 ok")
		case "RCPT":
			if s.failOn == "rcpt" {
				sendLine(550, "5.7.1 recipient denied")
				continue
			}
			to = extractAddr(args)
			sendLine(250, "2.1.5 ok")
		case "DATA":
			if s.failOn == "data" {
				sendLine(554, "5.7.1 transaction failed")
				continue
			}
			sendLine(354, "end data with <CR><LF>.<CR><LF>")
			for {
				dataLine, err := rw.ReadLine()
				if err != nil {
					return
				}
				if dataLine == "." {
					break
				}
				bodyBuf.WriteString(undot(dataLine))
				bodyBuf.WriteString("\r\n")
			}
			s.mu.Lock()
			s.messages = append(s.messages, receivedMessage{
				from: from,
				to:   to,
				body: []byte(bodyBuf.String()),
			})
			s.mu.Unlock()
			sendLine(250, "2.0.0 ok queued")
		case "QUIT":
			sendLine(221, "2.0.0 bye")
			return
		case "RSET":
			sendLine(250, "2.0.0 ok")
		case "NOOP":
			sendLine(250, "2.0.0 ok")
		default:
			sendLine(500, "5.5.2 unrecognized command")
		}
	}
}

// splitCmd splits "CMD arg1 arg2" into ("CMD", "arg1 arg2"),
// uppercasing the command. Returns ("", "") for empty input.
func splitCmd(line string) (string, string) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return "", ""
	}
	parts := strings.SplitN(line, " ", 2)
	cmd := strings.ToUpper(parts[0])
	if len(parts) == 1 {
		return cmd, ""
	}
	return cmd, parts[1]
}

// extractAddr pulls the bare address out of a "FROM:<a@b>" or
// "TO:<a@b>" argument. SMTP is the only place a "<...>" envelope
// shows up, and our tests only care about the address.
func extractAddr(arg string) string {
	start := strings.Index(arg, "<")
	end := strings.Index(arg, ">")
	if start < 0 || end < 0 || end <= start {
		return arg
	}
	return arg[start+1 : end]
}

// lastMessage returns the most recent message the server
// received, or nil if none. Used by tests to assert on the
// actual email body.
func (s *fakeSMTPServer) lastMessage() *receivedMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages) == 0 {
		return nil
	}
	m := s.messages[len(s.messages)-1]
	return &m
}

// countingDialer dials a fixed address and records that a dial
// happened. The actual SMTP work happens on the raw conn.
type countingDialer struct {
	addr  string
	dials int
}

func (d *countingDialer) Dial(ctx context.Context) (net.Conn, error) {
	d.dials++
	var d2 net.Dialer
	conn, err := d2.DialContext(ctx, "tcp", d.addr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// TestClient_Send_HappyPath exercises the full conversation
// against the fake server and asserts the recorded message has
// the right from/to and contains the body and subject.
func TestClient_Send_HappyPath(t *testing.T) {
	srv, addr := newFakeSMTPServer(t)
	dialer := &countingDialer{addr: addr}
	client, err := NewClient(ClientConfig{
		APIToken:    "test-token",
		FromAddress: "from@example.com",
		Host:        "localhost",
		Port:        strconv.Itoa(portFromAddr(addr)),
		Dialer:      dialer,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = client.Send(context.Background(), Message{
		To:      "to@example.com",
		Subject: "Hello",
		HTML:    "<p>body</p>",
		Text:    "body",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if dialer.dials != 1 {
		t.Errorf("expected 1 dial, got %d", dialer.dials)
	}
	got := srv.lastMessage()
	if got == nil {
		t.Fatal("server received no message")
	}
	if got.from != "from@example.com" {
		t.Errorf("from = %q, want from@example.com", got.from)
	}
	if got.to != "to@example.com" {
		t.Errorf("to = %q, want to@example.com", got.to)
	}
	if !strings.Contains(string(got.body), "Subject: Hello") {
		t.Errorf("body missing Subject header: %q", got.body)
	}
	if !strings.Contains(string(got.body), "<p>body</p>") {
		t.Errorf("body missing HTML part: %q", got.body)
	}
	if !strings.Contains(string(got.body), "multipart/alternative") {
		t.Errorf("body missing multipart/alternative header: %q", got.body)
	}
}

func TestClient_Send_TextOnly_OmitsMultipart(t *testing.T) {
	// When only the text body is set the message should be a
	// single text/plain part, not a multipart/alternative with
	// a missing html section. multipart-wrapping a single part
	// is a common but pointless footgun — some clients render
	// nothing if the html part is absent.
	srv, addr := newFakeSMTPServer(t)
	client, err := NewClient(ClientConfig{
		APIToken:    "test-token",
		FromAddress: "from@example.com",
		Host:        "localhost",
		Port:        strconv.Itoa(portFromAddr(addr)),
		Dialer:      &countingDialer{addr: addr},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = client.Send(context.Background(), Message{
		To:      "to@example.com",
		Subject: "Plain",
		Text:    "hello plaintext",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	body := string(srv.lastMessage().body)
	if strings.Contains(body, "multipart/alternative") {
		t.Errorf("text-only message should not be multipart, got: %q", body)
	}
	if !strings.Contains(body, "Content-Type: text/plain") {
		t.Errorf("text-only message missing text/plain part: %q", body)
	}
}

func TestClient_Send_ValidatesEmptyFields(t *testing.T) {
	// Validate up front, before opening a socket. The test
	// server is not even started — if validation does open a
	// socket, the test will block on Send until timeout.
	client, err := NewClient(ClientConfig{APIToken: "x"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	cases := []struct {
		name string
		msg  Message
	}{
		{"empty To", Message{Subject: "s", HTML: "h"}},
		{"empty Subject", Message{To: "a@b.c", HTML: "h"}},
		{"empty body", Message{To: "a@b.c", Subject: "s"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := client.Send(context.Background(), tc.msg)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestClient_Send_AUTHFailure(t *testing.T) {
	srv, addr := newFakeSMTPServer(t)
	srv.failOn = "auth"
	client, _ := NewClient(ClientConfig{
		APIToken:    "x",
		FromAddress: "from@example.com",
		Host:        "localhost",
		Port:        strconv.Itoa(portFromAddr(addr)),
		Dialer:      &countingDialer{addr: addr},
	})
	err := client.Send(context.Background(), Message{
		To: "to@example.com", Subject: "s", HTML: "h",
	})
	if err == nil {
		t.Fatal("expected AUTH error, got nil")
	}
	if !strings.Contains(err.Error(), "AUTH") {
		t.Errorf("expected error mentioning AUTH, got: %v", err)
	}
	if srv.lastMessage() != nil {
		t.Errorf("server should not have received a message after AUTH failure")
	}
}

func TestClient_Send_MailFromFailure(t *testing.T) {
	// 550 on MAIL FROM. Common when the from domain is not
	// onboarded for Email Sending. Client must surface the
	// error and not progress to RCPT/DATA.
	srv, addr := newFakeSMTPServer(t)
	srv.failOn = "mail"
	client, _ := NewClient(ClientConfig{
		APIToken:    "x",
		FromAddress: "from@example.com",
		Host:        "localhost",
		Port:        strconv.Itoa(portFromAddr(addr)),
		Dialer:      &countingDialer{addr: addr},
	})
	err := client.Send(context.Background(), Message{
		To: "to@example.com", Subject: "s", HTML: "h",
	})
	if err == nil {
		t.Fatal("expected MAIL FROM error, got nil")
	}
	if !strings.Contains(err.Error(), "MAIL FROM") {
		t.Errorf("expected error mentioning MAIL FROM, got: %v", err)
	}
}

func TestClient_Send_RcptToFailure(t *testing.T) {
	srv, addr := newFakeSMTPServer(t)
	srv.failOn = "rcpt"
	client, _ := NewClient(ClientConfig{
		APIToken:    "x",
		FromAddress: "from@example.com",
		Host:        "localhost",
		Port:        strconv.Itoa(portFromAddr(addr)),
		Dialer:      &countingDialer{addr: addr},
	})
	err := client.Send(context.Background(), Message{
		To: "to@example.com", Subject: "s", HTML: "h",
	})
	if err == nil {
		t.Fatal("expected RCPT TO error, got nil")
	}
	if !strings.Contains(err.Error(), "RCPT TO") {
		t.Errorf("expected error mentioning RCPT TO, got: %v", err)
	}
}

func TestClient_Send_DataFailure(t *testing.T) {
	srv, addr := newFakeSMTPServer(t)
	srv.failOn = "data"
	client, _ := NewClient(ClientConfig{
		APIToken:    "x",
		FromAddress: "from@example.com",
		Host:        "localhost",
		Port:        strconv.Itoa(portFromAddr(addr)),
		Dialer:      &countingDialer{addr: addr},
	})
	err := client.Send(context.Background(), Message{
		To: "to@example.com", Subject: "s", HTML: "h",
	})
	if err == nil {
		t.Fatal("expected DATA error, got nil")
	}
}

func TestClient_NewClient_RequiresAPIToken(t *testing.T) {
	// Empty token is a programming error, surfaced at
	// construction time. The SMTP server is not started
	// because the test relies on validation happening before
	// any network I/O.
	_, err := NewClient(ClientConfig{})
	if err == nil {
		t.Fatal("expected error for empty APIToken, got nil")
	}
	if !strings.Contains(err.Error(), "APIToken") {
		t.Errorf("expected error to mention APIToken, got: %v", err)
	}
}

func TestClient_NewClient_AppliesDefaults(t *testing.T) {
	// Empty FromAddress/Host/Port/DialTimeout must fall back
	// to the documented defaults. This is a guard against
	// someone refactoring the constructor and accidentally
	// dropping a default.
	c, err := NewClient(ClientConfig{APIToken: "x"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.cfg.FromAddress != defaultFrom {
		t.Errorf("FromAddress = %q, want %q", c.cfg.FromAddress, defaultFrom)
	}
	if c.cfg.Host != defaultSMTPHost {
		t.Errorf("Host = %q, want %q", c.cfg.Host, defaultSMTPHost)
	}
	if c.cfg.Port != defaultSMTPPort {
		t.Errorf("Port = %q, want %q", c.cfg.Port, defaultSMTPPort)
	}
	if c.cfg.DialTimeout != defaultDialTimeout {
		t.Errorf("DialTimeout = %v, want %v", c.cfg.DialTimeout, defaultDialTimeout)
	}
}

func TestClient_Send_DialFailureBubblesUp(t *testing.T) {
	// A dialer that always errors is the cleanest way to
	// exercise the dial-error branch without involving the
	// server. The error must wrap the underlying transport
	// error so operators can see the real cause.
	client, _ := NewClient(ClientConfig{
		APIToken: "x",
		Dialer:   failingDialer{err: errors.New("no route to host")},
	})
	err := client.Send(context.Background(), Message{
		To: "to@example.com", Subject: "s", HTML: "h",
	})
	if err == nil {
		t.Fatal("expected dial error, got nil")
	}
	if !strings.Contains(err.Error(), "no route to host") {
		t.Errorf("expected wrapped error to mention cause, got: %v", err)
	}
	if !strings.Contains(err.Error(), "dial") {
		t.Errorf("expected error to mention dial, got: %v", err)
	}
}

type failingDialer struct{ err error }

func (d failingDialer) Dial(ctx context.Context) (net.Conn, error) {
	return nil, d.err
}

// portFromAddr extracts the port from a "host:port" address.
// The fake server returns "127.0.0.1:54321"; we want "54321"
// to hand to the ClientConfig so it thinks the host is the
// local fake.
func portFromAddr(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(port)
	return n
}

// TestBuildMIME_SubjectEncodedForNonAscii guards the
// mime.QEncoding call on the Subject header. RFC 2047
// encoding is required for any non-ASCII subject so a
// Japanese or accented name does not break the recipient's
// mail client.
func TestBuildMIME_SubjectEncodedForNonAscii(t *testing.T) {
	body, err := buildMIME("from@example.com", Message{
		To: "to@example.com",
		// "Café" — the 'é' is non-ASCII; mime.QEncoding
		// should wrap this in =?utf-8?q?...?= per RFC 2047.
		Subject: "Café",
		HTML:    "<p>hi</p>",
	})
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	if !strings.Contains(string(body), "=?utf-8?") {
		t.Errorf("non-ASCII subject not encoded: %q", body)
	}
}

// TestBuildMIME_AsciiSubjectUnchanged guards the no-encoding
// fast path: ASCII subjects pass through unchanged so a
// human-readable subject survives a round-trip.
func TestBuildMIME_AsciiSubjectUnchanged(t *testing.T) {
	body, err := buildMIME("from@example.com", Message{
		To:      "to@example.com",
		Subject: "Welcome to Hylete",
		HTML:    "<p>hi</p>",
	})
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	if !strings.Contains(string(body), "Subject: Welcome to Hylete\r\n") {
		t.Errorf("ASCII subject not preserved verbatim: %q", body)
	}
}

// TestNewRawToken_UniqueAndBase64 guards the token generator
// (separate from the SMTP code so a regression in the random
// source is caught even if SMTP tests pass for unrelated
// reasons).
func TestNewRawToken_UniqueAndBase64(t *testing.T) {
	a, err := newRawToken()
	if err != nil {
		t.Fatalf("newRawToken a: %v", err)
	}
	b, err := newRawToken()
	if err != nil {
		t.Fatalf("newRawToken b: %v", err)
	}
	if a == b {
		t.Error("two calls produced the same token")
	}
	// 32 bytes -> 43 chars of base64url (no padding).
	if len(a) != 43 {
		t.Errorf("len(a) = %d, want 43", len(a))
	}
	if strings.ContainsAny(a, "+/=") {
		t.Errorf("token contains non-url-safe base64 chars: %q", a)
	}
}

// Compile-time check: the fake server is exercised via the
// client, so make sure countingDialer satisfies the Dialer
// interface and io.Reader is reachable through the conn.
var (
	_ Dialer = (*countingDialer)(nil)
	_        = io.Discard
	_        = bufio.NewReader
	_        = smtp.PlainAuth
)
