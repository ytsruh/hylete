// Package email sends transactional emails through Cloudflare's
// authenticated SMTP endpoint (smtp.mx.cloudflare.net:465). It hides
// the implicit-TLS SMTP details behind a small Client+Service pair
// so the rest of the codebase can call Service.SendWelcome without
// knowing anything about MIME, SASL, or hand-rolled TLS dials.
//
// Dependency boundary: the only third-party is the Cloudflare SMTP
// endpoint itself. net/smtp and crypto/tls are stdlib. Nothing in
// the rest of the app imports net/smtp — they import this package
// and the test suite fakes the network layer via a test-only Dialer
// injection.
package email

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Hard-coded Cloudflare endpoint values. These are part of the
// Cloudflare Email Sending contract and not configuration knobs:
// every Cloudflare account talks to the same SMTP server, on the
// same port, with the same auth identity.
const (
	defaultSMTPHost = "smtp.mx.cloudflare.net"
	defaultSMTPPort = "465"
	smtpUser        = "api_token"
	// defaultFrom is the from address used when ClientConfig.FromAddress
	// is empty. Pinned here per project policy: the sender domain is
	// hyleteapp.com (the same domain the app is deployed on), so any
	// future change to the From address is a code change, not a config
	// change.
	defaultFrom = "noreply@hyleteapp.com"
	// ehloIdentity is the hostname used in the EHLO greeting. SMTP
	// RFC 5321 §4.1.1.1 says it should be the client's FQDN. We
	// advertise the app's own hostname; some servers log it.
	ehloIdentity = "www.hyleteapp.com"

	defaultDialTimeout = 30 * time.Second
)

// Message is a single email to be sent. Subject, To, and at least
// one of HTML/Text are required; Send validates this and refuses
// to talk to the SMTP server otherwise.
type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

// Dialer dials the SMTP endpoint and returns an authenticated-ready
// net.Conn (TLS handshake done). The default implementation uses
// crypto/tls. Tests can supply a fake to skip real network I/O.
//
// The contract is: the returned conn must already have TLS done
// (for implicit-TLS port 465) and be ready to be wrapped in an
// smtp.NewClient call.
type Dialer interface {
	Dial(ctx context.Context) (net.Conn, error)
}

// ClientConfig holds the configurable knobs on Client. The zero
// value is not usable — APIToken must be set. All other fields
// fall back to safe defaults (Cloudflare's SMTP host/port, the
// pinned from address, a 30s dial timeout).
type ClientConfig struct {
	// APIToken is the Cloudflare API token used as the SMTP
	// password (username is the literal string "api_token"). The
	// token must have the "Email Sending: Edit" permission; this
	// is enforced by Cloudflare, not by this package.
	APIToken string

	// FromAddress is the envelope-from / From header. Defaults to
	// "noreply@hyleteapp.com" when empty.
	FromAddress string

	// Host is the SMTP server. Defaults to
	// "smtp.mx.cloudflare.net". Exposed primarily for tests so
	// they can point at a local fake server.
	Host string

	// Port is the SMTP port. Defaults to "465" (implicit TLS).
	Port string

	// DialTimeout bounds the TLS handshake. Defaults to 30s.
	DialTimeout time.Duration

	// Dialer is the network dialer used to reach the SMTP server.
	// Defaults to a crypto/tls-based dialer that targets Host:Port.
	// Tests inject a fake here to avoid real network I/O.
	Dialer Dialer
}

// Client sends Messages over Cloudflare's authenticated SMTP. It is
// safe for concurrent use: each Send call dials its own conn and
// closes it on return. There is no persistent connection pool —
// Cloudflare's session limits (50 RCPT per session) make per-send
// dials the simpler choice for a low-volume transactional sender.
type Client struct {
	cfg ClientConfig
}

// NewClient returns a Client ready to send. APIToken must be set;
// other fields fall back to package defaults. The defaults target
// Cloudflare's production endpoint; tests override Host + Dialer
// to point at a local fake.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.APIToken == "" {
		return nil, errors.New("email: ClientConfig.APIToken is required")
	}
	if cfg.FromAddress == "" {
		cfg.FromAddress = defaultFrom
	}
	if cfg.Host == "" {
		cfg.Host = defaultSMTPHost
	}
	if cfg.Port == "" {
		cfg.Port = defaultSMTPPort
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = defaultDialTimeout
	}
	if cfg.Dialer == nil {
		cfg.Dialer = &tlsDialer{
			host:    cfg.Host,
			port:    cfg.Port,
			timeout: cfg.DialTimeout,
		}
	}
	return &Client{cfg: cfg}, nil
}

// Send delivers msg through the SMTP server. The whole call is one
// session: dial TLS, EHLO, AUTH, MAIL, RCPT, DATA, QUIT. On any
// error the function returns a wrapped error and the underlying
// conn is closed.
//
// ctx controls only the dial; once the conn is established the
// SMTP conversation runs to completion. The stdlib smtp.Client
// does not expose per-command timeouts on a TLS conn, so a server
// that hangs mid-DATA will block until Go's transport-level
// deadlines fire. For the current traffic profile (a handful of
// emails per day) this is acceptable; revisit if the app ever
// bulk-sends.
func (c *Client) Send(ctx context.Context, msg Message) error {
	if err := validateMessage(msg); err != nil {
		return err
	}

	conn, err := c.cfg.Dialer.Dial(ctx)
	if err != nil {
		return fmt.Errorf("email: dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, c.cfg.Host)
	if err != nil {
		return fmt.Errorf("email: smtp client: %w", err)
	}
	// Close returns an error but Quit() (below) is the polite path;
	// Close is the fallback if Quit was never reached.
	defer client.Close()

	if err := client.Hello(ehloIdentity); err != nil {
		return fmt.Errorf("email: EHLO: %w", err)
	}

	auth := smtp.PlainAuth("", smtpUser, c.cfg.APIToken, c.cfg.Host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("email: AUTH: %w", err)
	}

	if err := client.Mail(c.cfg.FromAddress); err != nil {
		return fmt.Errorf("email: MAIL FROM: %w", err)
	}

	if err := client.Rcpt(msg.To); err != nil {
		return fmt.Errorf("email: RCPT TO: %w", err)
	}

	body, err := buildMIME(c.cfg.FromAddress, msg)
	if err != nil {
		return fmt.Errorf("email: build MIME: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email: DATA: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("email: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email: close body: %w", err)
	}

	// QUIT is best-effort. If the server already hung up (or our
	// local conn is half-closed) we still consider the send
	// successful — the server already accepted the message and
	// returned 250.
	_ = client.Quit()
	return nil
}

// validateMessage returns an error describing the first missing or
// invalid field. Splitting this out keeps Send's happy path linear.
func validateMessage(msg Message) error {
	if strings.TrimSpace(msg.To) == "" {
		return errors.New("email: Message.To is empty")
	}
	if strings.TrimSpace(msg.Subject) == "" {
		return errors.New("email: Message.Subject is empty")
	}
	if strings.TrimSpace(msg.HTML) == "" && strings.TrimSpace(msg.Text) == "" {
		return errors.New("email: Message must have HTML or Text body")
	}
	return nil
}

// tlsDialer is the default Dialer. It dials Host:Port with TCP,
// wraps the conn in TLS, and runs the handshake. The handshake is
// the actual blocking call; the TCP dial is bounded by timeout.
type tlsDialer struct {
	host    string
	port    string
	timeout time.Duration
}

func (d *tlsDialer) Dial(ctx context.Context) (net.Conn, error) {
	addr := net.JoinHostPort(d.host, d.port)
	dialer := &net.Dialer{Timeout: d.timeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	tlsConn := tls.Client(rawConn, &tls.Config{ServerName: d.host})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = rawConn.Close()
		return nil, err
	}
	return tlsConn, nil
}

// buildMIME assembles the bytes sent during the SMTP DATA phase:
// RFC 5322 headers + the message body. When both HTML and Text are
// provided, the body is multipart/alternative so the recipient's
// mail client picks the best part it can render. When only one is
// provided, it is sent as a single text/* part — no multipart
// wrapper, since wrapping a single part in multipart/alternative is
// a common but pointless footgun (it can cause some clients to
// render nothing).
//
// Content-Transfer-Encoding is set to 8bit. UTF-8 content goes
// through unmodified. Cloudflare's SMTP server advertises 8BITMIME
// in its EHLO response and accepts 8bit bodies; this is the same
// approach taken by the major transactional email providers.
func buildMIME(from string, msg Message) ([]byte, error) {
	hasHTML := strings.TrimSpace(msg.HTML) != ""
	hasText := strings.TrimSpace(msg.Text) != ""

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", from)
	fmt.Fprintf(&buf, "To: %s\r\n", msg.To)
	// mime.QEncoding returns the input verbatim for ASCII, and
	// wraps non-ASCII in =?utf-8?q?...?= per RFC 2047. Critical
	// for any non-ASCII subject (e.g. accents in user names).
	fmt.Fprintf(&buf, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", msg.Subject))
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")

	if !hasHTML {
		writeTextPart(&buf, msg.Text)
		return buf.Bytes(), nil
	}
	if !hasText {
		writeHTMLPart(&buf, msg.HTML)
		return buf.Bytes(), nil
	}

	boundary := randomBoundary()
	fmt.Fprintf(&buf, "Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary)
	fmt.Fprintf(&buf, "\r\n")

	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	writeTextPart(&buf, msg.Text)
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	writeHTMLPart(&buf, msg.HTML)
	fmt.Fprintf(&buf, "--%s--\r\n", boundary)

	return buf.Bytes(), nil
}

// writeTextPart writes the text/plain MIME part header + body into
// buf. Separated from buildMIME for symmetry with writeHTMLPart.
func writeTextPart(buf *bytes.Buffer, text string) {
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(text)
	if !strings.HasSuffix(text, "\r\n") {
		buf.WriteString("\r\n")
	}
}

// writeHTMLPart writes the text/html MIME part header + body into
// buf. Same shape as writeTextPart.
func writeHTMLPart(buf *bytes.Buffer, html string) {
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(html)
	if !strings.HasSuffix(html, "\r\n") {
		buf.WriteString("\r\n")
	}
}

// randomBoundary returns a 32-char hex string suitable for a MIME
// boundary. 128 bits of entropy is overkill (RFC 2046 only requires
// the boundary not to appear in any part) but cheap and makes
// accidental collisions impossible.
func randomBoundary() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is a system-level catastrophe;
		// falling back to a fixed string keeps the function
		// total without panicking in production.
		return "hylete-boundary"
	}
	return hex.EncodeToString(b[:])
}
