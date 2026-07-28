package email

type EmailService interface {
	SendInvitationEmail(toEmail string, token string, tenantName string) error
}
