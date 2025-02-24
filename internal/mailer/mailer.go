package mailer

import (
	"bytes"
	"embed"
	"text/template"
	"time"

	"github.com/go-mail/mail/v2"
)

// Below we declare a new variable with the type embed.FS(embed file system) to hold
// our mail templates. This has a directive in the format `//go:embed <path>`
// IMMEDIATELY ABOUT it, which indicates to Go that we want to store the contents of the
// templates directory in the variable templateFS. This will allow us to access the contents
// of the templates directory as a file system using the embed package.

//go:embed templates
var templateFS embed.FS

// Define a Mailer struct which contains a mail.Dialer instance(used to connect to an SMTP server),
// and the sender information for your emails (the name and address you want the emails to be from
// such as "Alice Smith <alice@example.com>").
type Mailer struct {
	dialer *mail.Dialer
	sender string
}

// Define a New function which initializes a new Mailer instance and returns a pointer to it.
func New(host string, port int, username, password, sender string) Mailer {

	// initialize a new mail.Dialer instance with the provided SMTP serve settings. we
	// also configure the dialer to use a 5-second timeout when connecting to the SMTP server.
	// This will prevent the application from hanging indefinitely if the SMTP server is not
	// available or is slow to respond.
	dialer := mail.NewDialer(host, port, username, password)
	dialer.Timeout = 5 * time.Second

	// return a new Mailer instance with the dialer and sender information
	return Mailer{
		dialer: dialer,
		sender: sender,
	}
}

func (m Mailer) Send(recipient, templateFile string, data interface{}) error {
	//use the ParseFS method to parse the email template file from the embedded file system
	// and return a new template.Template instance that we can use to render the email template.
	tmpl, err := template.New("email").ParseFS(templateFS, "templates/"+templateFile)
	if err != nil {
		return err
	}

	// Execute the named template "subject", passing in the dynamic data and storing the
	// result in a bytes.Buffer variable
	subject := new(bytes.Buffer)
	err = tmpl.ExecuteTemplate(subject, "subject", data)
	if err != nil {
		return err
	}

	// Execute the named template "plainBody", passing in the dynamic data and storing the
	// result in a bytes.Buffer variable
	plainBody := new(bytes.Buffer)
	err = tmpl.ExecuteTemplate(plainBody, "plainBody", data)
	if err != nil {
		return err
	}

	// same as above but for the "htmlBody" template
	htmlBody := new(bytes.Buffer)
	err = tmpl.ExecuteTemplate(htmlBody, "htmlBody", data)
	if err != nil {
		return err
	}

	// create a new mail.Message instance and set the recipient, sender, subject, and body of the email
	// using the values we generated from the email template above. We2q			 use the SetBody method to set the
	// plain text body of the email, and the AddAlternative method to add an HTML alternative body. This
	// allows email clients that support HTML to display the HTML version of the email, while clients that
	// do not support HTML will display the plain text version. It's important to note that AddAlternative
	// must be called after SetBody to ensure that the HTML version is correctly associated with the plain text version.
	msg := mail.NewMessage()
	msg.SetHeader("To", recipient)
	msg.SetHeader("From", m.sender)
	msg.SetHeader("subject", subject.String())
	msg.SetBody("text/plain", plainBody.String())
	msg.AddAlternative("text/html", htmlBody.String())

	// we will try to send the email up to 3 times if it fails. This is to handle temporary network issues or
	// SMTP server problems. If the email is sent successfully, we return nil. If it fails after 3 attempts, we
	// return the error. We also sleep for 500 milliseconds between each attempt to give the SMTP server a chance to recover.
	for i := 1; i <= 3; i++ {
		// use the dialer to connect to the SMTP server and send the email message then closes the connection. If
		// there is a timeout, it will return a "dial tcp: i/o timeout" error. or the associated error if there is one.
		err = m.dialer.DialAndSend(msg)
		// if the email was sent successfully, return nil
		if nil == err {
			return nil
		}

		// if it didnt work, sleep for 500 milliseconds and try again
		time.Sleep(500 * time.Millisecond)
	}

	return err
}
