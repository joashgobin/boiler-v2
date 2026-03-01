package email

import (
	"bytes"
	"database/sql"
	"embed"
	"fmt"
	"sync"
	"time"

	"github.com/wneessen/go-mail"

	ht "html/template"

	"github.com/gofiber/fiber/v3/log"
	"github.com/joashgobin/boiler-v2/helpers"
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

type MailInterface interface {
	Send(to, bcc, subject string, swaps ...any)
	NotifyAdmin(subject string, swaps ...any)
	GetMagicLink(email, purpose, urlPrefix string) string
	GetMagicLinks() []MagicLink
	IsMagicLinkValid(link string) bool
	SetMagicLinkResult(value, result string) string
}

func (m *MailModel) GetMagicLinks() []MagicLink {
	var links []MagicLink
	query := `
	SELECT id,email,purpose,value,result,used
	FROM magiclinks
	`
	rows, err := m.DB.Query(query)
	if err != nil {
		log.Errorf("magic links query error: %v", err)
		return nil
	}
	for rows.Next() {
		var link MagicLink
		err := rows.Scan(&link.ID, &link.Email, &link.Purpose, &link.Value, &link.Result, &link.Used)
		if err != nil {
			log.Errorf("magic links row scan error: %v", err)
			return nil
		}
		links = append(links, link)
	}
	return links
}

func (m *MailModel) SetMagicLinkResult(value, result string) string {
	updateQuery := `
	UPDATE magiclinks SET result = ? WHERE value = ?
	`
	stmt, err := m.DB.Prepare(updateQuery)
	if err != nil {
		log.Errorf("prepare statement error: %v", err)
		return ""
	}
	res, err := stmt.Exec(result, value)
	if err != nil {
		log.Errorf("execute error: %v", err)
		return ""
	}
	_, err = res.RowsAffected()
	if err != nil {
		log.Errorf("rows affected error: %v", err)
		return ""
	}

	var userEmail string
	err = m.DB.QueryRow(`
	SELECT email FROM magiclinks WHERE value = ?
	`, value).Scan(&userEmail)
	if err != nil {
		return ""
	}
	return userEmail
}

func (m *MailModel) IsMagicLinkValid(link string) bool {
	// log.Infof("verifying magic link: %s", link)
	updateQuery := `
	UPDATE magiclinks SET used = ? WHERE value = ?
	`
	stmt, err := m.DB.Prepare(updateQuery)
	if err != nil {
		log.Errorf("prepare statement error: %v", err)
		return false
	}
	defer stmt.Close()

	result, err := stmt.Exec(true, link)
	if err != nil {
		log.Errorf("execute error: %v", err)
		return false
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Errorf("rows affected error: %v", err)
		return false
	}
	if rowsAffected == 0 {
		return false
	}
	return true
}

func (m *MailModel) GetMagicLink(email, purpose, urlPrefix string) string {
	value := purpose + "_" + helpers.GetHash(email+purpose+time.Now().Format(time.RFC3339)) + "-" + helpers.GetRandomUUID()
	query := `
	INSERT INTO magiclinks(email,purpose,value,used,result) VALUES (?,?,?,?,?)
	`
	_, err := m.DB.Exec(query, email, purpose, value, false, "")
	if err != nil {
		log.Errorf("magic link generation error: %v", err)
		return urlPrefix
	}

	return urlPrefix + value
}

func (m *MailModel) NotifyAdmin(subject string, swaps ...any) {
	body := ""
	if len(swaps) > 1 {
		body = fmt.Sprintf(swaps[0].(string), swaps[1:]...)
	} else {
		body = swaps[0].(string)
	}
	SendEmail(helpers.Getenv("ADMIN_EMAIL"), subject, body, "", m.WaitGroup)
}

func (m *MailModel) Send(to, bcc, subject string, swaps ...any) {
	body := ""
	if len(swaps) > 1 {
		body = fmt.Sprintf(swaps[0].(string), swaps[1:]...)
	} else {
		body = swaps[0].(string)
	}
	query := `
	INSERT INTO mail (recipient, bcc, subject, body)
	VALUES (?,?,?,?)
	`
	result, err := m.DB.Exec(query, to, bcc, subject, body)
	if err != nil {
		log.Errorf("store email exec error: %v", err)
		return
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Errorf("store email rows error: %v", err)
		return
	}
	if rowsAffected == 0 {
		log.Errorf("store email error: no rows affected")
		return
	}
	SendEmail(to, subject, body, bcc, m.WaitGroup)
}

func SendEmail(to string, subject string, body string, bcc string, wg *sync.WaitGroup) {
	helpers.Background(
		func() {
			data := emailData{Subject: subject, Body: body}
			var senderAddr string = helpers.Getenv("MAIL_USER_EMAIL")
			var senderName string = helpers.Getenv("MAIL_USERNAME")
			username := senderAddr
			password := helpers.Getenv("MAIL_PW")
			mailHost := helpers.Getenv("MAIL_HOST")

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
