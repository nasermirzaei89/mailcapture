package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
)

type session struct {
	conn        net.Conn
	repo        MessageRepository
	logger      *slog.Logger
	reader      *bufio.Reader
	writer      *bufio.Writer
	remote      string
	greeted     bool
	hasMailFrom bool
	mailFrom    string
	rcptTo      []string
}

func newSession(conn net.Conn, repo MessageRepository, logger *slog.Logger) *session {
	return &session{
		conn:   conn,
		repo:   repo,
		logger: logger,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
		remote: conn.RemoteAddr().String(),
	}
}

func (s *session) run(ctx context.Context) {
	defer func() {
		_ = s.conn.Close()
	}()

	err := s.writeResponse(220, "mailcapture ready")
	if err != nil {
		s.logger.Error("smtp: greeting failed", "remote", s.remote, "error", err)
		return
	}

	for {
		line, err := s.readLine()
		if err != nil {
			if err != io.EOF {
				s.logger.Error("smtp: read command failed", "remote", s.remote, "error", err)
			}

			return
		}

		if line == "" {
			if writeErr := s.writeResponse(500, "empty command"); writeErr != nil {
				return
			}

			continue
		}

		cmd, arg := splitCommand(line)
		switch cmd {
		case "HELO", "EHLO":
			if strings.TrimSpace(arg) == "" {
				err := s.writeResponse(501, cmd+" requires domain/address")
				if err != nil {
					return
				}

				continue
			}
			s.greeted = true
			s.resetTransaction()

			err := s.writeResponse(250, "hello "+strings.TrimSpace(arg))
			if err != nil {
				return
			}
		case "NOOP":
			err := s.writeResponse(250, "ok")
			if err != nil {
				return
			}
		case "RSET":
			s.resetTransaction()
			err := s.writeResponse(250, "ok")
			if err != nil {
				return
			}
		case "QUIT":
			_ = s.writeResponse(221, "bye")
			return
		case "MAIL":
			if !s.greeted {
				err := s.writeResponse(503, "send HELO/EHLO first")
				if err != nil {
					return
				}

				continue
			}
			from, parseErr := parsePathArgument(arg, "FROM")
			if parseErr != nil {
				err := s.writeResponse(501, parseErr.Error())
				if err != nil {
					return
				}

				continue
			}
			s.hasMailFrom = true
			s.mailFrom = from
			s.rcptTo = s.rcptTo[:0]

			err := s.writeResponse(250, "ok")
			if err != nil {
				return
			}
		case "RCPT":
			if !s.hasMailFrom {
				err := s.writeResponse(503, "send MAIL FROM first")
				if err != nil {
					return
				}

				continue
			}

			to, parseErr := parsePathArgument(arg, "TO")
			if parseErr != nil {
				err := s.writeResponse(501, parseErr.Error())
				if err != nil {
					return
				}

				continue
			}

			s.rcptTo = append(s.rcptTo, to)

			err := s.writeResponse(250, "ok")
			if err != nil {
				return
			}
		case "DATA":
			if !s.hasMailFrom || len(s.rcptTo) == 0 {
				err := s.writeResponse(503, "need MAIL FROM and RCPT TO first")
				if err != nil {
					return
				}

				continue
			}
			err := s.writeResponse(354, "end with <CRLF>.<CRLF>")
			if err != nil {
				return
			}
			raw, dataErr := s.readData()
			if dataErr != nil {
				err := s.writeResponse(451, "failed to read DATA")
				if err != nil {
					return
				}

				continue
			}
			message, parseErr := parseMessage(s.mailFrom, s.rcptTo, raw)
			if parseErr != nil {
				err := s.writeResponse(451, "failed to parse DATA")
				if err != nil {
					return
				}

				continue
			}
			if _, storeErr := s.repo.Create(ctx, message); storeErr != nil {
				err := s.writeResponse(451, "failed to store message")
				if err != nil {
					return
				}

				continue
			}
			s.resetTransaction()

			err = s.writeResponse(250, "queued as "+message.ID)
			if err != nil {
				return
			}
		default:
			err := s.writeResponse(502, "command not implemented")
			if err != nil {
				return
			}
		}
	}
}

func (s *session) resetTransaction() {
	s.hasMailFrom = false
	s.mailFrom = ""
	s.rcptTo = s.rcptTo[:0]
}

func (s *session) readLine() (string, error) {
	line, err := s.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimRight(line, "\r\n"), nil
}

func (s *session) readData() ([]byte, error) {
	var buf bytes.Buffer
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "." {
			break
		}
		if strings.HasPrefix(trimmed, "..") {
			trimmed = trimmed[1:]
		}

		_, err = buf.WriteString(trimmed)
		if err != nil {
			return nil, err
		}

		_, err = buf.WriteString("\r\n")
		if err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func (s *session) writeResponse(code int, message string) error {
	_, err := fmt.Fprintf(s.writer, "%d %s\r\n", code, message)
	if err != nil {
		return err
	}

	return s.writer.Flush()
}

func splitCommand(line string) (string, string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", ""
	}

	parts := strings.SplitN(line, " ", 2)
	if len(parts) == 1 {
		return strings.ToUpper(parts[0]), ""
	}

	return strings.ToUpper(parts[0]), strings.TrimSpace(parts[1])
}

func parsePathArgument(arg string, keyword string) (string, error) {
	arg = strings.TrimSpace(arg)
	prefix := keyword + ":"
	if len(arg) < len(prefix) || !strings.EqualFold(arg[:len(prefix)], prefix) {
		return "", fmt.Errorf("expected %s:<address>", keyword)
	}

	rest := strings.TrimSpace(arg[len(prefix):])
	if rest == "" {
		return "", fmt.Errorf("missing %s address", strings.ToLower(keyword))
	}

	if strings.HasPrefix(rest, "<") {
		end := strings.Index(rest, ">")
		if end < 0 {
			return "", fmt.Errorf("invalid %s path", strings.ToLower(keyword))
		}
		address := strings.TrimSpace(rest[1:end])
		if keyword != "FROM" && address == "" {
			return "", fmt.Errorf("missing %s address", strings.ToLower(keyword))
		}

		return address, nil
	}

	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", fmt.Errorf("missing %s address", strings.ToLower(keyword))
	}

	return fields[0], nil
}
