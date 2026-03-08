package main

import (
	"bytes"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"time"
)

func parseMessage(envelopeFrom string, envelopeTo []string, raw []byte) (Message, error) {
	id, err := NewMessageID()
	if err != nil {
		return Message{}, err
	}

	message := Message{
		ID:           id,
		ReceivedAt:   time.Now().UTC(),
		EnvelopeFrom: envelopeFrom,
		EnvelopeTo:   append([]string(nil), envelopeTo...),
		Raw:          string(raw),
	}

	parsed, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		// Keep raw content even if parser cannot read headers.
		message.Body = string(raw)
		return message, nil
	}

	message.Subject = parsed.Header.Get("Subject")
	message.HeaderFrom = parsed.Header.Get("From")
	message.HeaderTo = splitHeaderList(parsed.Header.Get("To"))

	bodyBytes, readErr := io.ReadAll(parsed.Body)
	if readErr != nil {
		return Message{}, fmt.Errorf("read message body: %w", readErr)
	}
	message.Body = string(bodyBytes)

	return message, nil
}

func splitHeaderList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
