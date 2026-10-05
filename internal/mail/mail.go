package mail

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

// New returns an SMTP sender, or a sender that writes mail to the log when SMTP is not configured.
func New(cfg Config, logger *log.Logger) Sender {
	if cfg.Host == "" {
		return logSender{logger: logger}
	}
	return smtpSender{cfg: cfg}
}

type logSender struct{ logger *log.Logger }

func (l logSender) Send(_ context.Context, to, subject, body string) error {
	if _, err := netmail.ParseAddress(to); err != nil {
		return errors.New("invalid recipient address")
	}
	l.logger.Printf("component=mail status=not_configured to=%s subject=%q\n%s", to, subject, body)
	return nil
}

type smtpSender struct{ cfg Config }

func (s smtpSender) Send(ctx context.Context, to, subject, body string) error {
	msg, err := buildMessage(s.cfg.From, to, subject, body, time.Now())
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	if s.cfg.Port == 465 {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: s.cfg.Host}}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer c.Close()
	if s.cfg.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host}); err != nil {
				return fmt.Errorf("smtp starttls: %w", err)
			}
		}
	}
	if s.cfg.User != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.User, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	from, _ := netmail.ParseAddress(s.cfg.From)
	rcpt, _ := netmail.ParseAddress(to)
	if err := c.Mail(from.Address); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := c.Rcpt(rcpt.Address); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return c.Quit()
}

func buildMessage(from, to, subject, body string, now time.Time) ([]byte, error) {
	fromAddr, err := netmail.ParseAddress(from)
	if err != nil {
		return nil, errors.New("invalid SMTP_FROM address")
	}
	toAddr, err := netmail.ParseAddress(to)
	if err != nil {
		return nil, errors.New("invalid recipient address")
	}
	var b strings.Builder
	b.WriteString("From: " + fromAddr.String() + "\r\n")
	b.WriteString("To: " + toAddr.String() + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("Date: " + now.Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(body))
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	return []byte(b.String()), nil
}
