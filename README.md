# MailCapture

Catch emails locally while developing, then read them in a browser.

MailCapture is a Go app that gives you:

- an SMTP endpoint your app can send emails to
- a simple inbox UI to view received messages
- raw message view for debugging templates and headers

Perfect for local development and demos where you want to verify outgoing emails without sending anything to real users.

## Quick Start

Run the app:

```bash
go run github.com/nasermirzaei89/mailcapture@latest --smtp-addr :1025 --http-addr :8080
```

Open the inbox:

`http://localhost:8080`

Point your app's SMTP settings to:

- Host: `localhost`
- Port: `1025`
- Username/Password: not required

Once your app sends an email, it will appear in the inbox.

## What You Can Do

- See all captured emails in one place
- Open each message to inspect subject, sender, recipients, and body
- View raw source to debug headers and MIME output
- Catch multiple recipients in a single message

## Send a Quick Test Email

If you want to test immediately from terminal:

```bash
nc -C localhost 1025 <<'EOF'
HELO localhost
MAIL FROM:<from@example.com>
RCPT TO:<to@example.com>
DATA
Subject: Test email
From: from@example.com
To: to@example.com

Hello from MailCapture test.
.
QUIT
EOF
```

Refresh `http://localhost:8080` and you should see the message.

## Notes

- Emails are stored in memory only
- Restarting the app clears all captured messages
- Designed for local/dev usage only
- Not intended for production use or security-sensitive environments
