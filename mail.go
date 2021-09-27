package gomail

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"mime"
	"net/mail"
	"net/smtp"
	"path/filepath"
	"strings"
	"time"

	"github.com/t-okkn/go-enjaxel/io"
)

type SmtpServer struct {
	SmtpHost      string
	SmtpPort      uint16
	SmtpUsername  string
	SmtpPassword  string
	SmtpSecret    string
	Identity      string
}

type MailContent struct {
	From             *mail.Address
	To               []*mail.Address
	Cc               []*mail.Address
	Bcc              []*mail.Address
	ReplyTo          *mail.Address
	Subject          string
	Body             string
	BodyContentType  string
	Headers          map[string]string
	Attachments      map[string]string
	ServerInfo       *SmtpServer
}

func NewSmtpServer(
	host string,
	port uint16,
	username, password string,
) *SmtpServer {

	return &SmtpServer{
		SmtpHost: host,
		SmtpPort: port,
		SmtpUsername: username,
		SmtpPassword: password,
		SmtpSecret: "",
		Identity: "",
	}
}

func NewSmtpServerWithID(
	host string,
	port uint16,
	username, password, identity string,
) *SmtpServer {

	return &SmtpServer{
		SmtpHost: host,
		SmtpPort: port,
		SmtpUsername: username,
		SmtpPassword: password,
		SmtpSecret: "",
		Identity: identity,
	}
}

func NewSmtpServerCRAMMD5(
	host string,
	port uint16,
	username, secret string,
) *SmtpServer {

	return &SmtpServer{
		SmtpHost: host,
		SmtpPort: port,
		SmtpUsername: username,
		SmtpPassword: "",
		SmtpSecret: secret,
		Identity: "",
	}
}

func (s *SmtpServer) NewMessage(subject, body string) *MailContent {
	return s.newMessage(subject, body, "text/plain")
}

func (s *SmtpServer) NewHTMLMessage(subject, body string) *MailContent {
	return s.newMessage(subject, body, "text/html")
}

func (s *SmtpServer) EasySendMail(from, to, subject, body string) error {
	m := s.NewMessage(subject, body)

	if err := m.SetFrom(from); err != nil {
		return err
	}

	if err := m.AddTo(to); err != nil {
		return err
	}

	return m.SendMail()
}

func (m *MailContent) SetFrom(from string) error {
	return m.addMailAddress(from, "from")
}

func (m *MailContent) AddTo(to string) error {
	return m.addMailAddress(to, "to")
}

func (m *MailContent) AddCc(cc string) error {
	return m.addMailAddress(cc, "cc")
}

func (m *MailContent) AddBcc(bcc string) error {
	return m.addMailAddress(bcc, "bcc")
}

func (m *MailContent) SetReplyTo(reply string) error {
	return m.addMailAddress(reply, "reply")
}

func (m *MailContent) AddHeader(key, value string) error {
	if _, ex := m.Headers[key]; ex {
		e := errors.New("既に存在するキーです")
		return e
	}

	m.Headers[key] = value
	return nil
}

func (m *MailContent) SetHeader(key, value string) {
	m.Headers[key] = value
}

func (m *MailContent) Attach(file string) {
	_, filename := filepath.Split(file)
	m.Attachments[filename] = file
}

func (m *MailContent) SendMail() error {
	buf := bytes.NewBuffer(nil)
	boundary := "0141caffe046497"

	if m.From.Address == "" {
		e := errors.New("送信元を未指定にすることはできません")
		return e
	}
	buf.WriteString("From: " + m.From.String() + "\r\n")

	t := time.Now()
	buf.WriteString("Date: " + t.Format(time.RFC1123Z) + "\r\n")

	if len(m.To) == 0 {
		e := errors.New("宛先を指定してください")
		return e
	}
	buf.WriteString("To: " + getStringAddress(m.To) + "\r\n")

	if len(m.Cc) > 0 {
		buf.WriteString("Cc: " + getStringAddress(m.Cc) + "\r\n")
	}

	var enc64 = base64.StdEncoding
	subject := "=?UTF-8?B?" + enc64.EncodeToString([]byte(m.Subject)) + "?="
	buf.WriteString("Subject: " + subject + "\r\n")

	if m.ReplyTo.Address != "" {
		buf.WriteString("Reply-To: " + m.ReplyTo.String() + "\r\n")
	}

	if len(m.Headers) > 0 {
		for key, value := range m.Headers {
			buf.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
		}
	}

	if len(m.Attachments) > 0 {
		buf.WriteString("Content-Type: multipart/mixed; boundary=" + boundary + "\r\n")
		buf.WriteString("\r\n--" + boundary + "\r\n")
	}

	bct := fmt.Sprintf("Content-Type: %s; charset=utf-8\r\n\r\n", m.BodyContentType)
	buf.WriteString(bct)
	buf.WriteString(m.Body)
	buf.WriteString("\r\n")

	if len(m.Attachments) > 0 {
		for filename, abspath := range m.Attachments {
			if ok, _ := io.FileExists(abspath); !ok {
				e := errors.New("添付ファイルが存在しません")
				return e
			}

			buf.WriteString("\r\n\r\n--" + boundary + "\r\n")

			ext := filepath.Ext(filename)
			mimetype := mime.TypeByExtension(ext)

			if mimetype != "" {
				mimestr := fmt.Sprintf("Content-Type: %s\r\n", mimetype)
				buf.WriteString(mimestr)
			} else {
				buf.WriteString("Content-Type: application/octet-stream\r\n")
			}

			buf.WriteString("Content-Transfer-Encoding: base64\r\n")

			buf.WriteString("Content-Disposition: attachment; filename=\"=?UTF-8?B?")
			buf.WriteString(enc64.EncodeToString([]byte(filename)))
			buf.WriteString("?=\"\r\n\r\n")

			data, err := ioutil.ReadFile(abspath)
			if err != nil {
				return err
			}

			b := make([]byte, enc64.EncodedLen(len(data)))
			enc64.Encode(b, data)

			for i, max := 0, len(b); i < max; i++ {
				buf.WriteByte(b[i])
				if (i+1)%76 == 0 {
					buf.WriteString("\r\n")
				}
			}

			buf.WriteString("\r\n--" + boundary)
		}

		buf.WriteString("--")
	}

	s := m.ServerInfo
	var auth smtp.Auth

	if s.SmtpSecret == "" {
		auth = smtp.PlainAuth(
			s.Identity,
			s.SmtpUsername,
			s.SmtpPassword,
			s.SmtpHost,
		)

	} else {
		auth = smtp.CRAMMD5Auth(
			s.SmtpUsername,
			s.SmtpSecret,
		)
	}

	smtpaddr := fmt.Sprintf("%s:%d", s.SmtpHost, s.SmtpPort)

	return smtp.SendMail(
		smtpaddr,
		auth,
		m.From.Address,
		m.getToAddressList(),
		buf.Bytes(),
    )
}


func (s *SmtpServer) newMessage(
	subject, body, bodyContentType string,
) *MailContent {

	a := mail.Address{ Name: "", Address: "" }

	m := MailContent{
		From: &a,
		ReplyTo: &a,
		Subject: subject,
		Body: body,
		BodyContentType: bodyContentType,
		ServerInfo: s,
	}

	m.To = make([]*mail.Address, 0, 16)
	m.Cc = make([]*mail.Address, 0, 16)
	m.Bcc = make([]*mail.Address, 0, 16)

	m.Headers = make(map[string]string)
	m.Attachments = make(map[string]string)

	return &m
}

func (m *MailContent) addMailAddress(address, dest string) error {
	emails, err := mail.ParseAddressList(address)
	if err != nil {
		return err
	}

	switch dest {
	case "from":
		m.From = emails[0]

	case "to":
		for _, to := range emails {
			m.To = append(m.To, to)
		}

	case "cc":
		for _, cc := range emails {
			m.Cc = append(m.Cc, cc)
		}

	case "bcc":
		for _, bcc := range emails {
			m.Bcc = append(m.Bcc, bcc)
		}

	case "reply":
		m.ReplyTo = emails[0]

	default:
		e := errors.New("不正なアドレス追加先が検出されました")
		return e
	}

	return nil
}

func (m *MailContent) getToAddressList() []string {
	max := len(m.To) + len(m.Cc) + len(m.Bcc)
	tolist := make([]string, 0, max)

	for _, to := range m.To {
		tolist = append(tolist, to.Address)
	}

	for _, cc := range m.Cc {
		tolist = append(tolist, cc.Address)
	}

	for _, bcc := range m.Bcc {
		tolist = append(tolist, bcc.Address)
	}

	return tolist
}

func getStringAddress(mailobj []*mail.Address) string {
	strs := make([]string, 0, len(mailobj))

	for _, a := range mailobj {
		strs = append(strs, a.String())
	}

	return strings.Join(strs, ",")
}

