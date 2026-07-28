package email

import (
	"fmt"
	"os"

	"github.com/resend/resend-go/v2"
)

type ResendEmailService struct {
	client    *resend.Client
	fromEmail string
}

func NewResendEmailService(apiKey, fromEmail string) EmailService {
	if apiKey == "" {
		apiKey = os.Getenv("RESEND_API_KEY")
	}
	if fromEmail == "" {
		fromEmail = os.Getenv("RESEND_FROM_EMAIL")
	}

	client := resend.NewClient(apiKey)
	return &ResendEmailService{
		client:    client,
		fromEmail: fromEmail,
	}
}

func (s *ResendEmailService) SendInvitationEmail(toEmail string, token string, tenantName string) error {
	inviteURL := fmt.Sprintf("https://app.tutorin.com/invite?token=%s", token)

	htmlBody := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Undangan Bergabung</title>
    <style>
        body { font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif; background-color: #f4f6f8; margin: 0; padding: 40px 0; }
        .container { max-width: 600px; margin: 0 auto; background: #ffffff; border-radius: 8px; padding: 32px; box-shadow: 0 4px 12px rgba(0,0,0,0.05); }
        .header { font-size: 24px; font-weight: bold; color: #111827; margin-bottom: 16px; }
        .content { font-size: 16px; color: #374151; line-height: 1.6; margin-bottom: 24px; }
        .btn { display: inline-block; background-color: #2563eb; color: #ffffff !important; font-weight: 600; padding: 12px 24px; border-radius: 6px; text-decoration: none; font-size: 16px; }
        .footer { font-size: 12px; color: #9ca3af; margin-top: 32px; border-top: 1px solid #e5e7eb; padding-top: 16px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">Undangan Bergabung dengan %s</div>
        <div class="content">
            Halo,<br><br>
            Anda telah diundang untuk bergabung dengan bimbel <strong>%s</strong> di platform Tutorin.<br>
            Silakan klik tombol di bawah ini untuk menerima undangan dan menyelesaikan pendaftaran akun Anda:
        </div>
        <div style="text-align: center; margin: 32px 0;">
            <a href="%s" class="btn">Terima Undangan</a>
        </div>
        <div class="content" style="font-size: 14px; color: #6b7280;">
            Jika tombol di atas tidak berfungsi, salin dan tempel tautan berikut ke peramban Anda:<br>
            <a href="%s" style="color: #2563eb;">%s</a>
        </div>
        <div class="footer">
            Tautan undangan ini berlaku selama 48 jam.<br>
            &copy; Tutorin Platform. All rights reserved.
        </div>
    </div>
</body>
</html>
`, tenantName, tenantName, inviteURL, inviteURL, inviteURL)

	params := &resend.SendEmailRequest{
		From:    s.fromEmail,
		To:      []string{toEmail},
		Subject: fmt.Sprintf("Undangan Bergabung dengan %s - Tutorin", tenantName),
		Html:    htmlBody,
	}

	_, err := s.client.Emails.Send(params)
	return err
}
