package mail

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

// A relay that strips STARTTLS is the active-attacker version of a
// misconfigured relay. Neither the message nor the envelope may be sent.
func TestSendRefusesSMTPWithoutSTARTTLS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	command := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		_, _ = fmt.Fprint(conn, "220 local.test ESMTP\r\n")
		if _, readErr := reader.ReadString('\n'); readErr != nil { // EHLO
			return
		}
		_, _ = fmt.Fprint(conn, "250-local.test\r\n250 SIZE 1048576\r\n")
		if line, readErr := reader.ReadString('\n'); readErr == nil {
			command <- strings.TrimSpace(line)
		}
	}()

	address := listener.Addr().(*net.TCPAddr)
	sender := New(Config{
		Host: "127.0.0.1", Port: address.Port, From: "sender@example.com",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = sender.Send(ctx, Message{
		To: "reader@example.com", Subject: "Verify", Body: "secret link",
	})
	if !errors.Is(err, ErrTLSRequired) {
		t.Fatalf("Send error = %v, want ErrTLSRequired", err)
	}

	select {
	case line := <-command:
		t.Fatalf("SMTP command %q was sent after an insecure EHLO", line)
	case <-done:
	case <-ctx.Done():
		t.Fatal("SMTP test server did not observe the connection closing")
	}
}

// Exactly one layer may stuff a leading dot, and it is not compose: the
// writer Client.Data returns is a textproto.DotWriter, which does it. compose
// doing it as well sent ".." for a line beginning with ".", and the
// recipient's unstuffing removes only one — so the reader got a dot nobody
// typed. The test walks the body through the same writer the send path uses,
// because the invariant is about the two together.
func TestALeadingDotIsStuffedExactlyOnce(t *testing.T) {
	message := compose("sender@example.com", "reader@example.com", Message{
		Subject: "Verify",
		Body:    "first line\r\n.second line\r\n.",
	})
	// The body as it goes to the writer: everything after the header block.
	body := message[strings.Index(message, "\r\n\r\n")+4:]

	var wire bytes.Buffer
	writer := textproto.NewWriter(bufio.NewWriter(&wire)).DotWriter()
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	sent := wire.String()

	if !strings.Contains(sent, "\r\n..second line\r\n") {
		t.Errorf("a line beginning with one dot was not stuffed exactly once:\n%q", sent)
	}
	if strings.Contains(sent, "\r\n...") {
		t.Errorf("the leading dot was stuffed twice, so the reader gains a dot:\n%q", sent)
	}
	if !strings.Contains(sent, "\r\n..\r\n") {
		t.Errorf("a body line that is only a dot was not stuffed:\n%q", sent)
	}
	if !strings.HasSuffix(sent, "\r\n.\r\n") {
		t.Errorf("the message does not end with the bare terminator:\n%q", sent)
	}
}
