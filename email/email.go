package email

import (
	"bytes"
	"database/sql"
	"embed"
	"fmt"
	"sync"

	"github.com/wneessen/go-mail"

	ht "html/template"

	"github.com/gofiber/fiber/v3/log"
	"github.com/joashgobin/boiler-v2/email/cfg"
	"github.com/joashgobin/boiler-v2/helpers"
	resend "github.com/resend/resend-go/v4"
)

//go:embed templates/*
var templatesFS embed.FS

type emailData struct {
	Subject string
	Body    string
}

type MailModel struct {
	DB        *sql.DB
	WaitGroup *sync.WaitGroup
}

type MagicLink struct {
	ID      int
	Email   string
	Purpose string
	Value   string
	Result  string
	Used    bool
}

func NewMailModel(db *sql.DB, wg *sync.WaitGroup, appName string) *MailModel {
	helpers.MigrateUp(db, `
	USE <appName>;

CREATE TABLE IF NOT EXISTS magiclinks (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    email VARCHAR(100) NOT NULL,
    purpose VARCHAR(100) NOT NULL,
    value VARCHAR(200) NOT NULL UNIQUE,
    result VARCHAR(200) NOT NULL,
	used BOOLEAN
);

CREATE TABLE IF NOT EXISTS mail (
	id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
	recipient VARCHAR(1000) NOT NULL,
	cc VARCHAR(2000),
	bcc VARCHAR(2000),
	subject VARCHAR(1000) NOT NULL,
	body LONGTEXT NOT NULL
)
`, map[string]string{"appName": appName})
	return &MailModel{DB: db, WaitGroup: wg}
}

/*
func sendEmail(to string, subject string, body string, bcc string, wg *sync.WaitGroup) {
	helpers.Background(
		func() {
			data := emailData{Subject: subject, Body: body}
			var senderAddr string = helpers.GetEnv("MAIL_USER_EMAIL")
			var senderName string = helpers.GetEnv("MAIL_USERNAME")
			username := senderAddr
			password := helpers.GetEnv("MAIL_PW")
			mailHost := helpers.GetEnv("MAIL_HOST")

			message := mail.NewMsg()
			if err := message.FromFormat(senderName, senderAddr); err != nil {
				log.Infof("failed to set 'from' address: %s", senderAddr)
			}
			if err := message.To(to); err != nil {
				log.Infof("failed to set 'to' address: %s", to)
			}
			message.Subject(subject)
			message.SetBodyString(mail.TypeTextPlain, body)
			htmlTmpl, err := ht.New("").Funcs(ht.FuncMap{
				"safeHTML": func(s string) ht.HTML {
					return ht.HTML(s)
				},
			}).ParseFS(templatesFS, "templates/email.html")
			if err != nil {
				log.Infof("could not parse email template: %v", err)
				return
			}
			htmlBody := new(bytes.Buffer)
			err = htmlTmpl.ExecuteTemplate(htmlBody, "htmlBody", data)
			if err != nil {
				log.Infof("could not parse email template: %v", err)
				return
			}
			message.AddAlternativeString(mail.TypeTextHTML, htmlBody.String())
			message.AddBcc(bcc)

			client, err := mail.NewClient(mailHost, mail.WithTLSPortPolicy(mail.TLSMandatory),
				mail.WithSMTPAuth(mail.SMTPAuthPlain), mail.WithUsername(username), mail.WithPassword(password))
			if err != nil {
				log.Infof("failed to create mail client: %s\n", err)
			}

			if err := client.DialAndSend(message); err != nil {
				log.Infof("failed to send mail: %s", err)
			}

			// helpers.WasteTime(5)
			// log.Infof("Sending email\n'%s'\n to: %s", message, to)
		}, wg)
}
*/

type Mail struct {
	ID        int
	Recipient string
	CC        sql.NullString
	BCC       sql.NullString
	Subject   string
	Body      string
}

type MailInterface interface {
	Send(opts ...cfg.MailOption) error
	GetAll(filter string) []Mail
}

var _ MailInterface = (*MailModel)(nil)

func (m *MailModel) GetAll(filter string) []Mail {
	var mail []Mail
	query := `
	SELECT id,recipient,cc,bcc,subject,body
	FROM mail
	`
	rows, err := m.DB.Query(query)
	if err != nil {
		log.Errorf("get mail query error: %v", err)
		return nil
	}
	for rows.Next() {
		var newMail Mail
		err := rows.Scan(&newMail.ID, &newMail.Recipient, &newMail.CC, &newMail.BCC, &newMail.Subject, &newMail.Body)
		if err != nil {
			log.Errorf("get mail row scan error: %v", err)
			return nil
		}
		mail = append(mail, newMail)
	}
	return mail
}

func SendEmail(config cfg.MailConfig) {
	helpers.Background(
		func() {
			data := emailData{Subject: config.Subject, Body: config.Body}

			var senderAddr string = helpers.GetEnv("MAIL_USER_EMAIL")
			var senderName string = helpers.GetEnv("MAIL_USERNAME")

			password := helpers.GetEnv("MAIL_PW")
			mailHost := helpers.GetEnv("MAIL_HOST")
			username := senderAddr

			message := mail.NewMsg()
			// set email from and to
			if err := message.FromFormat(senderName, senderAddr); err != nil {
				log.Infof("failed to set 'from' address: %s", senderAddr)
			}
			if err := message.To(config.Recipients...); err != nil {
				log.Infof("failed to set 'to' address: %v", config.Recipients)
			}

			// set email subject and body string
			message.Subject(config.Subject)
			message.SetBodyString(mail.TypeTextPlain, config.Body)

			// parse html template
			htmlTmpl, err := ht.New("").Funcs(ht.FuncMap{
				"safeHTML": func(s string) ht.HTML {
					return ht.HTML(s)
				},
			}).ParseFS(templatesFS, "templates/email.html")
			if err != nil {
				log.Infof("could not parse email template: %v", err)
				return
			}

			// set email body
			htmlBody := new(bytes.Buffer)
			err = htmlTmpl.ExecuteTemplate(htmlBody, "htmlBody", data)
			if err != nil {
				log.Infof("could not parse email template: %v", err)
				return
			}
			message.AddAlternativeString(mail.TypeTextHTML, htmlBody.String())

			// add cc
			for _, c := range config.Cc {
				message.AddCc(c)
			}

			// add bcc
			for _, b := range config.Bcc {
				message.AddBcc(b)
			}

			// send email
			client, err := mail.NewClient(mailHost, mail.WithTLSPortPolicy(mail.TLSMandatory),
				mail.WithSMTPAuth(mail.SMTPAuthPlain), mail.WithUsername(username), mail.WithPassword(password))
			if err != nil {
				log.Infof("failed to create mail client: %s\n", err)
			}

			if err := client.DialAndSend(message); err != nil {
				log.Infof("failed to send mail: %s", err)
			}

			// helpers.WasteTime(5)
			// log.Infof("Sending email\n'%s'\n to: %s", message, to)
		}, config.WaitGroup)
}

func (m *MailModel) Send(opts ...cfg.MailOption) error {
	config := cfg.NewMailConfig(m.WaitGroup, opts...)

	if len(config.Recipients) == 0 {
		return fmt.Errorf("send mail error: no recipients specified")
	}
	if config.Subject == "" {
		return fmt.Errorf("send mail error: no subject specified")
	}
	if config.Body == "" {
		return fmt.Errorf("send mail error: no body specified")
	}

	recipients := fmt.Sprintf("%v", config.Recipients)
	bcc := ""
	if len(config.Bcc) > 0 {
		bcc = fmt.Sprintf("%v", config.Bcc)
	}

	query := `
	INSERT INTO mail (recipient, bcc, subject, body)
	VALUES (?,?,?,?)
	`
	result, err := m.DB.Exec(query, recipients, bcc, config.Subject, config.Body)
	if err != nil {
		return fmt.Errorf("store email exec error: %v", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store email rows error: %v", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("store email error: no rows affected")
	}
	SendEmail(config)
	return nil
}

type ResendMailer struct {
	client      *resend.Client
	wg          *sync.WaitGroup
	db          *sql.DB
	senderName  string
	senderEmail string
}

func (rm *ResendMailer) Send(opts ...cfg.MailOption) error {
	config := cfg.NewMailConfig(rm.wg, opts...)

	if len(config.Recipients) == 0 {
		return fmt.Errorf("send mail error: no recipients specified")
	}
	if config.Subject == "" {
		return fmt.Errorf("send mail error: no subject specified")
	}
	if config.Body == "" {
		return fmt.Errorf("send mail error: no body specified")
	}

	recipients := fmt.Sprintf("%v", config.Recipients)

	cc := ""
	if len(config.Bcc) > 0 {
		cc = fmt.Sprintf("%v", config.Cc)
	}
	bcc := ""
	if len(config.Bcc) > 0 {
		bcc = fmt.Sprintf("%v", config.Bcc)
	}

	query := `
	INSERT INTO mail (recipient, cc, bcc, subject, body)
	VALUES (?,?,?,?,?)
	`
	result, err := rm.db.Exec(query, recipients, cc, bcc, config.Subject, config.Body)
	if err != nil {
		return fmt.Errorf("store email exec error: %v", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store email rows error: %v", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("store email error: no rows affected")
	}

	params := &resend.SendEmailRequest{
		From:    fmt.Sprintf("%s <%s>", rm.senderName, rm.senderEmail),
		To:      config.Recipients,
		Html:    config.Body,
		Subject: config.Subject,
		Cc:      config.Cc,
		Bcc:     config.Bcc,
	}

	helpers.Background(func() {
		sent, err := rm.client.Emails.Send(params)
		if err != nil {
			log.Errorf("resend send email error: %v", err)
			return
		}
		log.Infof("resend send email success: %v", sent)
	}, rm.wg)
	return nil
}

func NewResendMailer(db *sql.DB, waitGroup *sync.WaitGroup) *ResendMailer {
	client := resend.NewClient(helpers.GetEnv("RESEND_API_KEY"))
	senderName := helpers.GetEnv("RESEND_SENDER_NAME")
	senderEmail := helpers.GetEnv("RESEND_SENDER_EMAIL")
	return &ResendMailer{
		client:      client,
		wg:          waitGroup,
		db:          db,
		senderName:  senderName,
		senderEmail: senderEmail,
	}
}
